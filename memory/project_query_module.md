---
name: project_query_module
description: Query module (FDRM) — in-memory matrix engine (prepare/execute) over form data; backend owns sources + SQL preview; cross-template links deferred.
metadata: 
  node_type: memory
  type: project
  originSessionId: 8defedf1-0690-4ec6-abec-0032ae644721
---

Query feature: a constrained, read-only SELECT surface over a template's form data. Studio dialog (Storage workspace, Data -> Query) + Wails service + REST `POST /api/collections/{tpl}/query`. **Re-architected 2026-05-29** away from the original index-backed design onto an in-memory matrix engine.

**Engine = prepare + execute (two steps), in `internal/modules/query`.** The user's direction: "make a full data matrix in memory and apply math on it"; "data is just data, all is string, types reapplied"; "two step prepare and execute".
- **`prepare` (prepare.go)** reads the template + its forms via a narrow `Loader` (app wires it over `template` + `storage` in `query_adapter.go`; tests use a fake) and flattens into a `Matrix`: one column per referenced source, scalar/facet values straight, referenced multi-valued fields (table columns, list/tags/multioption) **cartesian-exploded**. Reads form data directly, NOT the index — so ANY field is queryable (lifted the old `use_in_statistics`-only limit) and a table's columns arrive already row-aligned (the EAV index had no row index, which forced the old "one fanning column" guard and filter-folding hacks).
- **`execute` (matrix.go)** is pure string math over `[][]string`: filter (eq/ne text; lt/le/gt/ge coerce to number), project, distinct, multi-key sort (numeric or lexical), group + aggregate. Type is reapplied per operation, tolerantly.
- **Provenance (user's call): cartesian + "hash + position".** Each exploded row carries `Origin{Field, Row, Hash, Count}` per source table — a content hash (sha256/16B) for stable identity, the positional index so identical-content rows stay distinct. Aggregates `dedupeBySource` on (form, hash+position) so a cross-product from another table can't inflate `sum`/`avg` (each source row counts once). Two identical rows still both count (position differs).
- **Type integrity = surface, don't hide (user's principle).** Input enforces field types, so a typed (number/date) column must round-trip; a cell that won't is corruption, surfaced as `Result.Anomalies` (not silently skipped). Untyped columns have no expectation.

**Modes**: no GroupBy → row listing (distinct collapses the tuple). GroupBy → dimensions + `Measures []Measure{Func, Source, Header}` where Func ∈ count, count_distinct, sum, avg, min, max (`AggFuncs`). count = rows, count_distinct = distinct source forms.

**Backend owns the structural surface** (user: "the frontend should get the fields and possibilities from the backend"):
- `Service.Sources(template)` → `[]SourceInfo{ID, Label, Source, Numeric, Date, Fans, Aggregatable, Choices}` (sources.go). Replaced the deleted frontend `useQueryableSources.ts`.
- `Service.Explain(spec)` → SQL-shaped preview string rendered server-side (explain.go), so the SQL tab is authoritative, not a frontend approximation.
- `Service.FilterOps()`, `Service.Run(spec)`.

**Frontend `QueryDialog.vue`** (separate component): Tabs = Columns (drag-reorder via vuedraggable + global dnd kit) | Filters | Group (tick projected columns as group-by dimensions + aggregate measures) | Order (multi-level, drag priority, references output columns) | SQL (backend Explain). Anomalies surfaced as a banner. No SQL/derivation logic in TS. Default columns = all non-fanning sources + the first table's columns only (no accidental cross-product on open).

**Retired**: `index.ProjectRows` / `ProjectSpec` / filter-folding / `sameSource` / `orderClause` deleted from `internal/modules/index/aggregate_query.go`; kept the shared `colPred`/`colFilterCond`/`filterJoins`/`FilterOps` that `AggregateRaw` (stat) still uses. Query no longer imports `index` at all.

**Heavy testing** (user demanded "unit + feature, big matrices, all operations"): matrix_test.go + matrix_big_test.go (~27 unit, incl. 4k–10k-row matrices verified vs independent recomputation), prepare_test.go, sources_test.go, explain_test.go, query_test.go, and godog `features/matrix.feature` (8 scenarios). All race-clean.

It is intentionally NOT a DBMS: single-template, no cross-template joins, no subqueries, no user SQL.

**FDRM (functional data relations management)** is the user's north-star for the eventual cross-template link: model a relation as a *function* declared on the source side (the `api` field type already is `record -> {records in template B by GUID}`), not a join table. The content hash per row is a foundation for content-addressed cross-references.

**FDRM staged north-star** (user's direction):
1. single-template matrix query — DONE (this module).
2. cross-template functional link in the READ path — resolve the `api` field live or materialize a `form_links` table at reconcile. The deferred "join".
3. **Virtual templates** = read-join + write-router: a declared composition owning no storage, routing each field's value back to its rightful storage item on save. Two unsolved design problems first: (a) write atomicity across files (no cross-file transaction; needs best-effort rollback or surfaced partial-write); (b) create cardinality (1:N links become sub-lists). Build only after rung 2 + a concrete use case. [[project_api_module]] [[feedback_atomic_writes]] [[feedback_backend_owns_data]] [[project_composite_stats]] [[project_scaling_stats]]
