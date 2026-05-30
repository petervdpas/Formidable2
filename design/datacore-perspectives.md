# Datacore: perspectives over a hybrid substrate

Status: conceptual frame. Drafted 2026-05-29. Not a migration plan, not a
commitment to build. This is the lens to check the next stat / query / FDRM
decision against, so those choices stop being made one engine at a time.

Relates to the matrix query engine ([[project_query_module]]), the SQLite
index ([[project_index_form_reader]], [[project_fulltext_search]]), the
statistical engine ([[statistics-dsl]]), and the cross-template FDRM
direction recorded in [[project_query_module]].

## The question this settles

Statistics runs on the SQLite index (EAV `form_values`); query runs on the
in-memory matrix that reads forms directly. They diverged onto two engines.
The recurring question is "should stat move to the matrix too," and every
time it gets answered as an engine choice: in-memory vs database, flexibility
vs scale. That framing is wrong, and it keeps producing forks.

The reframe: a tensor is not an engine, it is a lens. The data is stored
once. What stat and query produce are different projections of it. Seen that
way, they were never two engines, they are two families of perspective over
one substrate.

## Two layers, reasoned about separately

### Logical: the perspective vocabulary

Borrow the axes from the `(I, F, M)` data-tensor model (prototyped in the
local UniBase repo, `internal/tensor`). They map onto Formidable almost
one to one:

| Axis / op | Meaning | Formidable |
|---|---|---|
| project along **I** | which records | forms / GUIDs |
| project along **F** | which fields | template fields |
| scope along **M** | which context | facets (context-varying meaning) |
| **follow** a ref | step to a related record | FDRM cross-template link |
| **reduce** along an axis | aggregate | count / sum / distribution |

A small algebra falls out: select records, project fields, scope context,
follow, reduce. Both query and stat get spelled in it.

- a query result is a perspective (select + project + filter + sort)
- a distribution is a perspective (project one field, reduce by count)
- a cross-tab is a perspective with two axes pinned
- a date series is a perspective bucketed along a temporal scope
- an FDRM relation is a perspective that follows a ref into another template

Stat shapes are not a separate engine, they are named perspectives. This is
the clean statement of consolidation: not "rewrite stat as reducers over the
matrix," but "stat and query are projections in the same vocabulary."

Discipline: keep the vocabulary tiny. It is worth formalizing only because
three consumers will speak it (query, stat, FDRM). If it were one, it would
be over-engineering. Resist it growing into a bespoke query language before a
second caller actually needs an operator.

### Physical: hybrid, and an execution detail

Storage stays hybrid because find/join/scan and compute want different
machinery, and that is fine:

| Engine | Good at | Role |
|---|---|---|
| SQLite index | indexed lookup, FTS5 search, facet filter, key-join pushdown | narrow and find: which records, where |
| in-memory matrix | row-aligned table data, type integrity, provenance, math over a narrowed set | compute: what is in them, what the numbers say |

The split is real but it is an execution concern, not an architecture fork. A
planner decides, per operation, what to push down to the index (narrowing,
FTS, key-joins, anything that wants a B-tree) and what to materialize into
memory and compute (the math over the narrowed set). The perspective
definition does not know or care which side ran it. That decoupling is the
whole point: the index-vs-memory call becomes a planner optimization, not a
decision stat and query each have to make for themselves.

The planner is the seam between the two layers.

### The seam, built (2026-05-30)

The first cut of the planner exists in code, additive and read-only. The shape:

- `datacore.Predicate` is a narrowing request: facet equality, scalar field
  equality, full-text search. Empty means "no narrowing, build everything".
- `datacore.Planner` is the seam interface: `Plan(template, pred) -> (ids,
  narrowed, err)`. `narrowed=false` means "not pushable, fall back to a full
  build", so a missing planner or an unanswerable predicate is always correct,
  just unaccelerated.
- `datacore.SubsetLoader` lets the loader materialize only the narrowed ids.
  A plain loader still works (load all, filter), so the seam never forces the
  interface on a fixture or a future source.
- `buildNarrowed` ties them together: planner narrows which records exist, the
  tensor computes over them. The reducers gain `*Where` variants
  (`CountWhere`, `DistributionWhere`) that carry a predicate.

The index side is `datacoreIndexPlanner` (composition root): full-text Search
hits FTS5, Facet conditions filter the indexed facet rows, field Equals hits a
new `index.FormsWithValue` query over `form_values`. Conditions are
intersected (the predicate is an AND). The contract is parity-tested: narrowing
through the real SQLite index produces the same answer as selecting the same
set in memory with `Where` over the full tensor, for facet, scalar, and
two-condition-interaction predicates, plus the empty, no-overlap, and
concurrent paths.

What this unblocks: stat and query can now push a filter down to the index and
compute the narrowed set in the tensor, instead of either reading every form
(datacore today) or being capped by EAV (the index today). That is the
prerequisite for routing stat or query through datacore without a performance
regression. Those migrations are not started.

## Why the flags survive

`use_in_statistics` stays, unchanged in purpose. On the index it does double
duty: it is both the author's "these fields are meant to be measured" signal
and the technical gate that decides whether a field is even reachable. The
perspective frame separates those. The flag remains the focus signal (stat
leads with flagged fields, defaults to them, treats them as the curated
surface). It stops being a capability boundary, because a perspective can
project any field. You keep the curation; you lose the cage. Same logic
applies to the index's one-table-column and no-cross-table limits: those are
EAV's decisions, not yours, and the perspective layer does not inherit them.

## What UniBase contributed, and the bar to graduate

UniBase (local repo) is where the `(I, F, M)` + `Ref` model came from, and
that model is the gift: cross-template links as first-class refs you follow,
context as a real axis (it already matches facets), self-describing schema.
It is a working prototype of the data model FDRM should use. Steal the model.

Do not adopt the engine yet. In its current form it is a sparse associative
store, not a numeric tensor: `Query.Run()` full-scans the cell map, there is
no value index, the whole tensor lives in RAM with whole-file load/save, and
one global lock wraps the scan. That is the matrix's scaling profile with
less optimization than SQLite already gives us. To earn the datacore seat it
would need value indexing (so queries are not full scans), paging (so it is
not all-in-RAM), and a benchmark proving it beats the SQLite + matrix combo
at real Formidable sizes. Until then it is a research substrate and a model
reference, not a dependency.

Note on the name: UniBase's "tensor" is a 3-axis addressable store, not a
linear-algebra tensor. There is no contraction or vectorized reduction to
inherit; aggregation is still hand-written loops. The value is the axes (the
perspectives), not the storage.

## The scaling reality this frame respects

The base corpus is nothing in memory. What blows up is combinatorial, and it
blows up in any decade:

- within one template, cross-referencing several multi-row tables fans
  cartesian (`t1 x t2 x t3 x t4`). `prepare` already fans only referenced
  tables, which contains it, but it is latent.
- across templates, an FDRM key-join done naively in memory is a nested-loop
  cartesian. That is precisely the workload to push down to an indexed store,
  not to materialize.

The planner respects this directly: joins and narrowing go to the index, the
matrix only ever materializes the narrowed working set. Perspectives stay
honest about scale because the physical layer underneath them is allowed to
be smart.

## What this is not

Not a decision to migrate stat off the index now. The index limits are not
biting today; they limit the *future* of statistics (unflagged fields, a
second table column, cross-template). When that future arrives, this frame
says how to take it: express the new shape as a perspective, let the planner
place it, do not fork a third engine. The trigger to act is the first stat
feature that wants an unflagged field, a second table column, or a
cross-template reach. Build then, against this frame, not before.
