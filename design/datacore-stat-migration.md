# Stat on datacore: migration plan

Status: planned, not started. Needs an explicit go before any production code,
because it touches the working stat module. This file lines up the work so the
swap is a series of small, parity-gated steps rather than a rewrite.

## Goal, and what stays

Move stat's *compute* off the SQLite index and onto the datacore tensor, while
the index keeps its narrowing job (find/filter/FTS, the planner seam). Stat
stops reading EAV aggregates and reads perspectives instead. Nothing about the
chart output, the DSL, or the stat Service changes. The flags
(`use_in_statistics`) stay as the focus signal.

Non-goal: removing the index. It remains the narrowing store and keeps
satisfying `stat.Index` so we can run both and compare.

## The seam

`stat.Manager` depends on one interface, `stat.Index` (`internal/modules/stat/stat.go`):

| Method | datacore perspective |
|---|---|
| `TotalForms(t)` | `Count(t, "")` |
| `ValueDistribution(t, key, col)` | `Distribution(key)`; col set: `Follow(table).Distribution(colKey)` |
| `NumericValues(t, key, col)` | `Aggregate(key).Values`; col set: `Follow(table).Aggregate(colKey).Values` |
| `FacetDistribution(t, key)` | `Distribution("facet:"+key)` |
| `FacetCross(t, a, b)` | `Cross("facet:"+a, "facet:"+b)` |
| `DateSeries(t, key, col, period)` | `DateSeries(key, period)`; col set: followed |
| `AggregateRaw(t, dims, nums, filters)` | `Grid(...)` |

`*index.Manager` satisfies the interface today. The migration is one new type
that satisfies it with datacore behind it. The interface is typed in `index.*`
structs (`index.Bucket`, `index.CrossCell`, `index.AggDim/AggNum/AggFilter`,
`index.StatRawRow`), so the adapter translates datacore results back into those
shapes. That keeps `stat.Manager` untouched.

## Strategy: adapter, not rewrite

1. New `datacoreStatIndex` (composition root, `internal/app`) implementing
   `stat.Index`. It holds the datacore loader factory (or `*datacore.Service`)
   and the template manager (for the column bridge below). Each method builds a
   tensor for the template, runs the perspective, maps the result to the
   `index.*` type stat expects.
2. Swap behind a config flag (`stat_engine: index | datacore`, default
   `index`). `app.go` picks which `stat.Index` to hand `stat.NewManager`. One
   line changes; everything downstream is identical.
3. Flip only when the parity gate is green (below). Removing the index's
   compute methods is a later, separate step, never part of the flip.

## The column bridge (the one real translation)

`stat.Index` speaks the index's positional column language: a table column is
`(fieldKey, col *int)`. datacore speaks named columns: `(Table, Field)`. The
adapter needs the reverse of the existing `statColumnResolver.ColumnIndex`
(key to index): an index-to-key lookup over the template's table field
`Options` (position `col` to its value-key). One small helper,
`columnKeyIn(t, fieldKey, col) (string, bool)`, mirrors `columnIndexIn`. With
it:

- `col == nil` to a root field (`Field=key, Table=""`).
- `col != nil` to a table column (`Table=key, Field=columnKey`), reached by
  `Follow(key)`.

For `AggregateRaw`, the same translation maps each `index.AggDim`/`AggNum`:
`Kind=facet` to `Field="facet:"+key`; `Col` set to `Table=key, Field=colKey`;
`DateWidth` carries straight over. `index.StatRawRow` maps back from `GridRow`
(`Form`, `Dims`, `Nums` as `sql.NullFloat64` from `NumCell`).

## Known divergences and edges to settle before the flip

1. **Table-column fan is an intended divergence, not a parity miss.** Two
   columns of one table: the index cartesians (its own doc tells the stat
   engine to allow only one table source); datacore aligns per row. Proven by
   `TestDatacore_TableColumnFanAlignsWhereIndexCartesians`. The flip makes a
   previously-blocked stat (two aligned table columns) correct. This must be
   called out, not "fixed" to match the index.

2. **FacetDistribution and the set-but-unselected bucket. SETTLED: accept the
   divergence.** The index groups a facet that is set with no option chosen
   under the empty `""` label ("(unset)"), via its `set_flag=1` +
   `COALESCE(selected,'')` join. This reaches both the direct
   `FacetDistribution`/`FacetCross` methods and the DSL path (the facet-dim join
   in `AggregateRaw`). datacore drops it: blank is absence, uniformly for fields
   and facets (the substrate ruling), so it does not manufacture the "(unset)"
   category. This is an intended divergence, like the table-column fan, not a
   parity miss. Flipping the engine drops the "(unset)" facet category from
   distributions and crosses that have unselected facets; that is the accepted
   behavior change. Pinned by `TestStatAdapter_FacetUnsetBucketDiverges` (direct
   methods) and `TestStatDSLFacetUnsetDiverges` (DSL path); both assert the
   engines still agree on every real category. The loader's facet-skip line
   carries a comment pointing here. Note: this is the same reason field
   distributions already match (the index field-dim join also excludes blanks);
   only facets carry the asymmetry.

3. **Per-call tensor rebuild.** datacore builds a fresh tensor per `stat.Index`
   call; `NumericStats` and `CrossTab` each make two calls, so two builds. The
   planner seam does not help here (these are template-wide). Acceptable for the
   first cut; optimize later with a per-request tensor cache keyed on
   (template, rev) if it shows up. Note it, do not pre-optimize.

## Parity gate: test it like crazy

The whole migration is a bet that two engines agree. That bet is only as good
as the tests behind it, so the gate is exhaustive by design. Nothing flips
until every layer below is green under `-race`.

Already green (`internal/app/datacore_stat_crosscheck_test.go`):
ValueDistribution, FacetCross, NumericValues (and `.Values`), DateSeries,
AggregateRaw core, and the table-column divergence.

**Layer 1, per-method parity (extend the crosscheck file).** One test per
`stat.Index` method, index result vs datacore result on a shared fixture,
compared as the method's natural shape (multiset where order is undefined).
Add the two missing: `TotalForms` (`Count` vs `TotalForms`) and
`FacetDistribution` (after edge 2 is decided). Each method also gets its
unhappy and boundary cases, not just the clean one:
- empty template (no forms), single form, all-blank field
- missing field, missing facet, missing template
- a non-coercible value in a numeric field (anomaly path), a non-date in a date
  field (dropped from the series)
- blank vs absent (the field unset vs set to "")
- a table column reached by `col != nil`, including an empty table and a row
  missing the column
- date period year/month/day, and a value on a period boundary

**Layer 2, DSL parity through `stat.Manager` (the real gate).** DONE,
`internal/app/datacore_stat_dsl_parity_test.go`. Two managers,
`stat.NewManager(idxImpl)` and `stat.NewManager(datacoreImpl)`, same
`SourceOptions` + `ColumnResolver`, the *same DSL strings* through both, assert
identical `*Grid` (axes, labels, measures, cells keyed by coord, total,
percents within tolerance for float-add order). 15 DSL queries (scalar/facet/
date-bin distributions, numeric measures incl. median/stddev, records, pct
forms, field + facet filters, single table-column dim, table-column measure,
rank-2 mixed) plus the 8 convenience methods (Distribution / FacetDistribution
/ TimeSeries / NumericStats incl. percentile / CrossTab). Key finding: the
engine's fan-out guard rejects two table-column sources, so the cartesian
divergence is unreachable through the DSL; through stat the engines agree on
everything, including a single table-column dimension. Scaled and composite
parity DONE too (`datacore_stat_scaled_composite_parity_test.go`):
`EvaluateScaled` over five DSLs incl. the weighted `pct forms` denominator (the
153% bug area), and `EvaluateComposite` plain and scaled-parent, comparing the
parent grid plus every branch child grid.

**Layer 3, randomized parity (property test). DONE,
`internal/app/datacore_stat_property_test.go`.** 60 seeded fixtures (seed = loop
index, so a failure names the exact case): 3-14 forms with a status text field,
an amount number (sometimes blank, sometimes junk), a due date (sometimes
absent), two always-set facets, and a 0-3 row items table. Each fixture asserts
index == datacore for TotalForms, ValueDistribution (scalar + table column),
NumericValues (scalar + table column), FacetDistribution, FacetCross, DateSeries
(year/month/day), and AggregateRaw (scalar + facet dims, num, filter). The
generator avoids the two settled divergences (facets always non-empty, no
two-table-column cross), so any disagreement is a real bug.

**Layer 4, the deliberate divergences (assert they differ, correctly).** The
table-column fan and (if we accept it) the facet empty-bucket are *meant* to
differ. Each gets a test that pins datacore's value to a hand-computed correct
expectation and documents that the index is the one that is wrong. A divergence
must never be silent: if a fanning case slips into a parity test it should fail
loudly, which is why fanning is partitioned out of layers 2 and 3.

**Layer 5, race + concurrency. DONE** (`datacore_stat_property_test.go`,
`TestStatProperty_ConcurrentDatacoreManager`). 32 goroutines fire five DSL
queries through the datacore-backed manager at once, each result checked against
the index's answer computed up front; the whole suite runs under `-race`. Proves
the engine is safe for the real concurrent-request pattern (each call builds its
own tensor, nothing shared mutably).

**Optional, shadow mode for live verification.** A `stat.Index` wrapper that
calls the index (authoritative, returns its result) and datacore in parallel,
compares, and `slog`s any mismatch tagged with the method and template, with the
fanning cases whitelisted as expected-divergence. Run it in a real session
before flipping the flag: real templates exercise paths no fixture will. This
is verification on top of the gate, not a replacement for it.

## Rollout

1. Build `datacoreStatIndex` + `columnKeyIn`. No wiring yet.
2. Test layer 1 (per-method parity + unhappy/boundary) green under `-race`.
3. Settle edge 2 (facet empty bucket); add its parity test.
4. Test layer 2 (DSL parity through `stat.Manager`, both engines) green: the
   real gate.
5. Test layers 3-5 (randomized parity, divergence pins, concurrency) green.
6. Add the `stat_engine` flag, default `index`. Wire the picker in `app.go`.
7. Optional shadow run on real templates; review the `slog` mismatch tail.
8. Flip to `datacore` per-profile for live use once the gate is clean; keep the
   index path as the fallback.
9. Later, separate change: retire the index's compute methods (the `aggregate*`
   files) once datacore has driven stat in production with no surprises.

No step past 1 touches anything a user sees until step 8, and that step is one
flag with the index still a fallback. Steps 2-5 are pure test work: the flip is
not even on the table until they are all green.

## Why this order

The adapter is additive and testable with the index still authoritative. The
flag makes the flip reversible. The two-engine parity test is the safety net.
Nothing the user sees changes until step 5, and that step is one flag.
