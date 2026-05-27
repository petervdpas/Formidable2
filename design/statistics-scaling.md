# Scaling statistical objects (weighted measures)

Status: built. Shipped 2026-05-27. Builds on the Statistical Engine
([[statistics-dsl]]), the `records()` measure ([[project_records_measure]]),
and the named-object catalog introduced for composites
([[statistics-composite]]).

A scaling object is a reusable weighting. On its own it draws nothing; it is
referenced by name from another object, which then weights each contributing
unit by a factor drawn from a per-form categorical value instead of adding 1.

The motivating case (ODS Uitfasering): rank applications not by raw heaviness
(how many in-use tables each touches) but by *urgency*. A table whose FCDM
coverage is poor is more work to retire, so it should count heavier. Define a
weighting on the `fcdm` facet (`AANWEZIG` -> 0.5, `NIET AANWEZIG` -> 2,
default 1) and apply it to the per-application `records()` measure. An app
sitting in many low-coverage tables now scores high.

## What a scaling is, in one line

A scaling turns the unit contribution of `count()` / `records()` from a
constant 1 into a factor looked up from each form's value of a chosen
categorical source. The measure becomes a weighted sum (a score).

- `records()` + scale: for each group, sum one factor per **distinct form**.
- `count()` + scale: for each group, sum one factor per **row**.

Numeric reduces (`sum`, `avg`, ...) are untouched; weighting only has meaning
for the counting measures.

## Why it is its own object, not an inline map

The weight map is small but it is shared. The same FCDM-coverage weighting
applies to several charts (apps, stored procedures, views). Inlining the map
into each chart's DSL would fork the source of truth: change one factor and you
must hunt every chart. So a scaling is a named object, exactly like a composite
parent, and consuming objects reference it:

```
records() by F["code-repositories"]["application"] top 10
  where Facet["flag"] eq "IN GEBRUIK"
  scale "fcdm-urgency"
```

The DSL carries only the name. The referenced object owns the source and the
map. Edit `fcdm-urgency` once, every referencing chart updates. This mirrors
how a composite carries parent/child names, not inlined configs.

## The per-form rule

A scaling source must be **per-form**: a facet (one selected option per form)
or a scalar dropdown/radio field (one value per form). A table-column source is
rejected, because its value fans out per row and has no single per-form weight.

This is what makes `records()` + scale well defined: each distinct form has
exactly one factor, so summing one factor per distinct form is unambiguous.
(For `count()` the unit is the row, and every row of a form shares that form's
factor, so it stays well defined too.)

A form that has no value for the source (facet unset, field blank) falls to the
scaling's **default factor**, so an unset form is neutral (default 1) rather
than silently dropped or zeroed. The builder seeds every listed option to 1, so
the map is explicit.

## Object model

A scaling is a source plus an ordered option-to-factor map plus a default. It
is stored as a `statistics:` entry with a `scaling:` block, mutually exclusive
with `dsl:` and `composite:` per object (the three object kinds).

```yaml
statistics:
  - name: fcdm-urgency
    label: "FCDM coverage urgency"
    scaling:
      source:
        kind: facet            # "facet" | "field"
        key: fcdm
      weights:
        - { label: "AANWEZIG", factor: 0.5 }
        - { label: "GEDEELTELIJK AANWEZIG", factor: 1 }
        - { label: "IN VOORBEREIDING", factor: 1.5 }
        - { label: "NIET AANWEZIG", factor: 2 }
      default: 1

  - name: gas-apps
    label: "GAS applicaties (urgency-weighted)"
    dsl: >-
      records() by F["code-repositories"]["application"] top 10
      where Facet["flag"] eq "IN GEBRUIK" scale "fcdm-urgency"
```

The weight `label` is the **stored value** the form carries for that source: a
facet's selected option label, or a dropdown/radio field's option `value`.
Those are the same strings the dimension/filter machinery already compares
against, so the closed-set option list the builder offers and the keys the
engine looks up agree by construction.

## Engine

`Manager.EvaluateScaled(template, cfg, *Scaling)` is `Evaluate` with an optional
weighting; `Evaluate` is now `EvaluateScaled(..., nil)`. When a scaling is
present:

1. Validate the source is per-form (reject a table column).
2. Resolve a `form -> factor` lookup. A second, unfiltered `AggregateRaw` over
   just the scale source (`formCategory`) gives each form's category; forms
   missing it are absent (INNER JOIN), so the default factor applies.
3. In the group reducer, accumulate the weighted sum: per row for `count()`
   (`g.wcount += factor`), and per distinct form for `records()` (sum the
   factor over the group's form set, reusing the `records()` form tracking).

Top-N then ranks by the weighted value (urgency), which is the desired order.
Percentages (`pct`) compute over the weighted values like any other.

No new SQL and no row-shape change: the per-form factor is resolved with one
extra aggregate over the existing index, then applied in Go. Backend steers;
the math is server-side so every renderer reads one figure.

## Resolution and where it lives

The DSL scale clause is a name; the Service resolves it. `EvaluateObject` and
`EvaluateDSL` (the builder preview) build the template's object catalog, look up
the named scaling, and call `EvaluateScaled`. An unknown scale name is an error,
not a silent unweighted run. A scaling object has no grid of its own, so
evaluating one directly errors (REST returns 404; the list omits its eval href).

Composite children honor scale too. `ResolveComposite` resolves each child's
(and the parent's) scale-clause name into a `*Scaling` and attaches it to the
`Edge` (and `Composite`), and `EvaluateComposite` evaluates through
`EvaluateScaled`. So a drilled child is weighted exactly as it is standalone; a
child that names a scale the source cannot resolve is an error, never a silent
unweighted ring. (Earlier the child evaluated through `Manager.Evaluate`, which
dropped the clause; the composite ring showed raw counts while the standalone
object showed weighted sums.)

## Builder and renderer

- Scaling builder (`ScalingBuilderModal`): compact single pane. Pick a per-form
  source (facets + dropdown/radio fields, via the same closed-set option
  discovery the WHERE filter uses), then a factor per option and a default. The
  option set is closed and short, so a flat label/factor list beats a
  master-detail layout.
- Statistics builder: a `scale` block (next to the percentage-base block) is a
  dropdown of the template's scaling objects plus "no weighting", shown only
  when the template has at least one scaling. It sets `cfg.Scale`.
- Renderer: none. A weighted measure is still a rank-N grid; existing
  bar/pie/heatmap renderers draw it unchanged. The values are weighted sums
  rather than counts; the chart label conveys the meaning.

## Scope, in retrospect

- DSL: a trailing `scale "<name>"` clause (round-trips, canonical order before
  `pct`, quoted name so hyphens survive).
- Engine: `Scaling` type, `EvaluateScaled`, `formCategory`, weighted count /
  records.
- Resolution: `StatObject.Scaling`, `catalogConfigs.Scaling`, Service wiring.
- Storage: `template.StatScaling` / `StatSource` / `StatWeightEntry`, normalize
  keep-rule, plugin-adapter mapping.
- REST: `kind:"scaling"` in the catalog listing, no eval href, 404 on direct
  evaluate.
- Frontend: scaling builder, scale block in the stat builder, list row, i18n
  (en + nl).

No new aggregation primitive: a scaling reuses the existing per-form values and
the `records()` form tracking, adding one resolution hop and a multiply.

## Decisions (settled)

- Default factor for an unlisted / unset option: 1 (neutral). The builder
  pre-fills every option at 1 so the map is explicit.
- One scale clause per object, applying to its counting measures. Per-measure
  weighting is unnecessary for the single-measure case and can be added later.
- Source restricted to per-form facet / dropdown / radio; table columns
  rejected (no single per-form weight).
- Weights live on the scaling object (the DSL/config), never on the template's
  field/facet definitions, so they stay per-statistic and do not edit
  author-owned field options.
