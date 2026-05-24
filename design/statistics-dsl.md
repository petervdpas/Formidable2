# Statistics DSL - design

Status: design, not built. Decided 2026-05-24.

A template-authored, presentation-free statistics layer. The author composes
named **statistical objects** (a small DSL) in a builder. Each object, when
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
dimension := source bin?
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
3. Engine: StatConfig -> SQL GROUP BY over form_values/form_facets -> Grid.
   Tests over a seeded index (mixed field + facet dimensions, each measure).
4. Wails `Stat.EvaluateObject(template, name) -> Grid` + Lua binding
   (`formidable.statistical(name)`).
5. Builder dialog (mirrors expressionBuilder structure).
6. Renderer: extend StatChart to consume Grid (rank-aware), add pie/heatmap.

## Deferred

- `where` filters (reserved keyword).
- Numeric-range binning for number dimensions (only date binning in v1).
- Rank > 2 rendering (engine produces it; renderer flattens what it can show).
