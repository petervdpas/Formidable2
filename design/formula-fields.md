# Formula fields (computed fields)

Status: built. Shipped 2026-06-01. Builds on the expression engine
([[render-helpers]] catalog and the `sidebar_expression` evaluator), the
datacore substrate (design/datacore-stat-migration.md), and the statistics
engine (design/statistics-dsl.md).

A formula field is a named, reusable, per-record computed value, in the spirit
of Crystal Reports formula fields. The author writes an expression once; it
becomes an ordinary field that statistics can group by or aggregate.

## One line

A formula turns an expression like `(F["architectuur"] == "HOOG" ? 10 : 1) *
F["fcdm-dekking"]` into a field `F["heaviness"]`, computed per record, usable
anywhere a real field is, e.g. `sum(F["heaviness"]) by F["applicatie-naam"]`.

## Where it lives: the datacore loader, not the index

Production statistics read **datacore**, not the SQLite index
(`stat.NewManager(statengine.New(datacoreSvc, ...))`). Datacore is an in-memory
`(I, F, M)` tensor whose loader builds cells from live forms. A formula field is
just a derived field whose cells are computed in the loader. Once a formula's
value is a cell, every datacore perspective (group-by, sum, distribution,
cross) treats it as an ordinary field, so there is no index materialization and
no perspective-algebra change. This is the datacore "meaning in the loader"
ruling: datacore stays a pure cells+refs substrate; the meaning (the
expression) is applied by the loader.

Concretely the loader adapter (`internal/app/datacore_adapter.go`), after
building a record from the form, calls `applyFormulas`: it builds the
evaluation context from the **typed** form data (numbers stay `float64` so
arithmetic is real, not string concatenation) plus the set facets, evaluates
each formula in declared order (a later formula may reference an earlier one via
`F["..."]`), coerces the result to the declared type, and writes it as a field
cell. A formula that fails to evaluate is skipped (no cell), matching the
loader's read-tolerance.

## The expression engine, elevated

The engine previously returned only a styled `Result` (text/color/classes), for
the sidebar. Formulas need the raw scalar, so the engine gained
`EvaluateRaw` / `Manager.EvaluateValue`, which run the same compile + helpers
but skip the Result normalisation and return the typed value
(number/string/bool). `coerceFormula` then renders it to the string cell
datacore stores, handling `int`/`int64`/`float64` uniformly.

## Object model

A formula is `{ key, label, type, expression }` stored as a `formulas:` entry on
the template, mutually independent of `statistics:` and `facets:`. `type` is the
result coercion: `number | text | date | bool`. The key shares the `F["key"]`
namespace with fields and facets, so validation rejects a collision.

```yaml
formulas:
  - key: heaviness
    label: "Heaviness (architectuur x fcdm)"
    type: number
    expression: >-
      (F["architectuur"] == "HOOG" ? 10 : F["architectuur"] == "MIDDEL" ? 5 :
       F["architectuur"] == "LAAG" ? 1 : 0) * F["fcdm-dekking"]
```

## Builder and preview

- A **Formulas** tab on the template editor lists the catalog; `FormulaEditorModal`
  authors one (key, label, type, expression) with `F["key"]` reference chips and
  a live preview that evaluates against the template's first stored form through
  `FormulaService.Preview` (the backend builds the same context the chart will,
  so the preview is the real value).
- The statistics builder offers each formula as a source, so it appears in the
  measure/dimension pickers like any field.

## Scope, in retrospect

Built: the catalog, the loader hook, the typed engine path, the tab + editor +
preview, formula fields as stat sources. Deferred (none require a redesign, each
is a new consumer of the same catalog): display in the storage list / on the
form, CSV export columns, the Query (FDRM) module, a full formula -> formula
dependency DAG (declared order only for now), per-table-row formulas, and
conditional formatting (color/classes, the way `sidebar_expression` already
does).

## Decisions (settled)

- Formulas evaluate in the loader adapter (composition root), never in
  datacore's pure ingest, so datacore owns no opinion about expressions.
- The evaluation context is the typed form data, not the stringified datacore
  cells, so numeric arithmetic works.
- Declared-order referencing (not a full DAG) covers the common case; a later
  formula sees earlier ones.
- The default result type is `number` (the only one that aggregates
  numerically); a blank type normalises to it.
