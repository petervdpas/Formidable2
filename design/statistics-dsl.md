# Statistical Engine - DSL design

Status: design + DSL built (steps 1-2). Decided 2026-05-24.

The Statistical Engine is a template-authored, presentation-free statistics
layer. The author composes named **statistical objects** (a small DSL) in a
builder. Each object, when
evaluated, returns an N-dimensional grid of values. Lua/plugins fetch an
object by name, evaluate it, and render it however they like. Analysis reports
reuse the same path later.

This is NOT the expression DSL. Expression is predicate/row-wise (one record ->
a styled chip), with no grouping or aggregates. Stats needs GROUP BY over the
whole collection, so it is a fresh DSL + engine. It borrows only expression's
*patterns*: a serialized string with compile->parse->compile round-trip
identity, a visual builder, and a `Manager.Evaluate`-style engine.

## Layers

1. **Sources** = the template's statistical fields (`use_in_statistics`; table
   columns via `statistics_columns` value-keys) + its facets (always sources).
   This is the already-shipped opt-in layer ([[project_stats_opt_in]]).
2. **Statistical objects** = named DSL strings stored on the template (plural).
   Pure data spec: dimensions (group-by) x measures (aggregate) -> rank-N grid.
3. **Engine** = a GROUP BY query builder over `form_values` + `form_facets`.
   The existing fixed aggregates (ValueDistribution, NumericValues,
   FacetDistribution, FacetCross, DateSeries) become special cases.
4. **Renderer** = chart-neutral; the frontend maps rank-0 -> cards, rank-1 ->
   bar/pie, rank-2 -> stacked/heatmap. No chart type lives in the object.
5. **Consumers** = Lua/plugins now, analysis reports later. Same object, engine,
   renderer; different surface.

## Template extension

```yaml
statistics:
  - name: by-status
    label: "By status"
    dsl: 'count() by F["status"]'
  - name: amount-by-region-year
    label: "Amount by region and year"
    dsl: 'sum(F["amount"]) by F["region"], F["due"]@year'
```

```go
// internal/modules/template/types.go - new on Template, beside Facets
Statistics []Statistic `yaml:"statistics,omitempty" json:"statistics"`

type Statistic struct {
    Name  string `yaml:"name"  json:"name"`   // identifier, used by Lua + display
    Label string `yaml:"label,omitempty" json:"label,omitempty"`
    DSL   string `yaml:"dsl"   json:"dsl"`     // the serialized statistical DSL
}
```

## Grammar

```
object    := measure ("," measure)*  ( "by" dimension ("," dimension)* )?
measure   := "count" "(" ")"
           | reduce "(" numSource ")"
           | "percentile" "(" numSource "," number ")"
reduce    := "sum" | "avg" | "min" | "max" | "median" | "stddev"
dimension := source bin? ( "top" number )?     // top N: keep N biggest, 1..20
source    := "F[" str "]" ( "[" str "]" )?      // field, or table column by value-key
           | "Facet[" str "]"                    // facet
numSource := "F[" str "]" ( "[" str "]" )?       // must resolve to numeric field/column
bin       := "@year" | "@month" | "@day"         // date sources only
str       := JSON-quoted string (same escaping as expression's F["..."])
```

Decisions taken:
- Table column reference = **bracket chain**: `F["items"]["qty"]` (field key,
  then column value-key). Resolved to the positional `col` index at eval via
  the template Options (same map normalizeStatisticsColumns uses).
- **Multiple measures** allowed in v1 (`count(), avg(F["amount"]) by ...`). Each
  measure is a value layer per cell. Measure vocabulary is extensible: count,
  sum, avg, min, max, median, stddev, percentile, ... ("distribution" is just
  `count()` over a dimension, i.e. the rank-1 count).
- **top N** (generic, any dimension): keep the N categories with the highest
  first-measure total, ranked desc, **drop the tail** (Total stays the full
  form count). Range 1..20 (engine clamps; Compile rejects out of range).
  Mitigates a high-cardinality axis, e.g. `count() by F["base-table"] top 10`.
  Builder prefills 10 for a text-field dimension; others default to no cap.
- **Filters** (a `where` clause to scope which forms count) are **deferred**;
  the keyword is reserved.

Examples mapped to shape:

```
count() by F["status"]                              # rank-1 array
count() by Facet["priority"], Facet["stage"]        # rank-2 matrix
count() by F["due"]@month                            # rank-1 time array
avg(F["amount"]) by F["status"]                      # rank-1, measure over a field
sum(F["items"]["qty"]) by F["region"], F["due"]@year # rank-2, table column + date bin
count(), avg(F["amount"]) by F["status"]             # rank-1, two measure layers
count()                                              # rank-0 scalar
```

## Output: rank-N grid, sparse cells

```go
type Grid struct {
    Axes     []Axis   `json:"axes"`      // one per dimension, declared order
    Measures []string `json:"measures"`  // "count", "avg(amount)", ...
    Cells    []Cell   `json:"cells"`     // sparse
    Total    int      `json:"total"`     // denominator for %
}
type Axis struct {
    Source string   `json:"source"`      // dimension source label
    Labels []string `json:"labels"`      // distinct category ticks
}
type Cell struct {
    Coords []int     `json:"coords"`     // index into each axis
    Values []float64 `json:"values"`     // one per measure
}
```

Sparse `Cells` (coords + values) instead of nested arrays: dense N-D arrays are
awkward in Go/JSON and a matrix is usually sparse. The renderer densifies for
rank-1/2.

## Round-trip contract

`Compile(Config) -> string`, `Parse(string) -> Config`, with
compile->parse->compile **string equality** (the expression module's contract).
The builder parses the stored DSL into a Config, edits, compiles back. Strict
parse: unrecognised shapes fail and the builder surfaces a clear "couldn't load"
flow rather than silently misreading.

## Lua

Fetch an object by name (scoped to the ctx/active template) and get its
evaluated grid:

```lua
local g = formidable.statistical("by-status")   -- template.statistical.<name>
-- g = { axes = {...}, measures = {...}, cells = {...}, total = N }
```

The grid is presentation-free; the plugin renders it. The chart-neutral
frontend renderer (StatChart family) is the default visualiser the plugin can
hand the grid to.

The DSL + engine live in `internal/modules/stat` (the "statistical engine"),
alongside the chart-neutral `Result` it generalizes. Go type is `StatConfig`
(disambiguated from the package's `Result`); funcs `stat.Compile(StatConfig)`
and `stat.Parse(string)`.

## Build order (backend-first, TDD)

1. DONE - `Statistic` type on Template (`statistics:` section) + Normalize
   (trim, drop empty, dedupe by name; DSL parsing deferred to the engine to
   keep template decoupled). Tests in template/normalize_test.go.
2. DONE - DSL `stat.Compile`/`stat.Parse` + `StatConfig` types + round-trip
   identity tests (stat/dsl*.go, stat/dsl_test.go).
3. DONE - Engine: `stat.Manager.Evaluate(template, StatConfig) -> Grid`.
   `index.AggregateRaw` fetches one row per form (scalar field + facet
   dims, date bins, LEFT-joined numeric sources); `stat` groups + reduces
   in Go (count + Summarize for sum/avg/min/max/median/stddev/percentile)
   into sparse Grid cells. Table-column sources rejected (deferred). Tests:
   index/aggregate_grid_test.go (real SQL) + stat/engine_test.go (shaping).
4. Wails `Stat.EvaluateObject(template, name) -> Grid` + Lua binding
   (`formidable.statistical(name)`).
5. DONE - Builder dialog. `StatisticsBuilderModal.vue` composes measures
   + dimensions (sources = the template's use_in_statistics fields/columns
   + facets) and round-trips the DSL string via `Stat.CompileDSL` /
   `Stat.ParseDSL`. The op/bin catalog + input rules come from the backend
   (`Stat.BuilderMeasureOps` / `BuilderBins`); the UI is a backend-driven
   string-builder, not a DSL re-implementation. Managed from a new
   "Statistics" tab in the template editor (TemplatesWorkspace). i18n ns
   statistics.json (en+nl). (Step 4, Lua/Wails Evaluate, intentionally
   still pending - the builder only needs Compile/Parse.)
6. DONE - Renderer. `components/stat/grid.ts` (local Grid type +
   densify helpers) + rank-aware components: StatGridScalars (rank-0),
   StatGridBar / StatGridPie (rank-1), StatGridHeatmap (rank-2), dispatched
   by StatGrid.vue; StatGridDialog wraps them with a chart-type toggle
   (bar/pie on rank-1) + measure picker. Wired as a "View" button on the
   Statistics tab (calls Stat.EvaluateDSL on the draft DSL, so it works
   before save). Rank > 2 shows "nothing to render" (deferred). Stat
   service also gained EvaluateDSL for this preview path.

## Table-column sources (lifted, guarded)

Table-column sources `F["table"]["col"]` are supported for the cases that
are exactly correct, and rejected for the ones that would over-count via
the fan-out of a one-to-many join:
- allowed: a single table-column **dimension** with `count()` (optionally
  crossed with scalar/facet dims, which attribute per cell), e.g.
  `count() by F["stored-procedures"]["access"]`; and a `sum`/`avg`/... over
  a table-column **numeric source** when the dimensions are scalar/facet.
- rejected (clear error): two table-column sources; or a numeric measure
  alongside a table-column dimension.
Mechanics: `index.AggDim.Col` / `AggNum.Col` add column-indexed joins;
`stat.ColumnResolver` (app: `statColumnResolver` over the template field
options) maps the DSL column key to the positional `form_values.col`.

## Deferred

- `where` filters (reserved keyword).
- Numeric-range binning for number dimensions (only date binning in v1).
- Rank > 2 rendering (engine produces it; renderer flattens what it can show).
- Fixed-option set + labels for table-column dimensions (a table dropdown
  column still shows present values, not its full choice set / captions).
