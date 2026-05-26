# Composite statistical objects (hop routes)

Status: design only. Drafted 2026-05-26. Not built. Depends on the shipped
Statistical Engine ([[statistics-dsl]]) and the `records()` measure
([[project_records_measure]]).

A composite object links existing statistical objects into a hierarchy: a
parent distribution whose branches drill into child objects. It draws as a
nested/sunburst chart where one branch can expand and a sibling can stay a
solid leaf.

The motivating case (ODS Uitfasering): "In Gebruik" splits the records into
IN GEBRUIK (71) / NIET IN GEBRUIK (47); the user wants the IN GEBRUIK slice
subdivided by the applications that hit those in-use records, while NIET IN
GEBRUIK stays a solid red leaf.

## What does NOT work, and why

Combining the *outputs* of two independent objects cannot produce the
nesting. Each object is a **margin**: already aggregated, with the joint
information thrown away.

- "Applications" (unfiltered) = app totals over all 118 records. FMU = 45.
- "In Gebruik" = the record split, 71 / 47.

From those two outputs alone you cannot know how many of FMU's 45 mentions
fall in in-use records. It could be 45, it could be 5. That split lives only
in the raw records, not in either margin. Reconstructing a joint from its
margins is not possible in general.

So "compose two output grids" would mislead: users expect nesting and would
get a margin product.

## What does work: hops that carry records

A hop carries the **record set** forward, not the aggregated grid, and
re-aggregates at each step:

1. Hop 1: group the 118 records by `flag` -> branches {IN GEBRUIK: 71 records},
   {NIET IN GEBRUIK: 47 records}.
2. Hop 2: take the 71 records of the green branch and group *those* by
   application.

Because hop 2 re-derives from the actual subset, it recovers the joint. This
is strictly more expressive than a flat cross-tab (`count() by flag,
application`): a cross-tab expands every cell into a full matrix (heatmap),
whereas a hop route expands one branch and leaves siblings as leaves
(sunburst). The asymmetry (green expands, red solid) is the whole point.

## The relation that makes it sound

A child object already encodes which branch it belongs to, through its filter:

```
parent = In Gebruik              # axis: flag -> IN GEBRUIK | NIET IN GEBRUIK
child  = Applications            # has: ... where flag eq "IN GEBRUIK"
edge   = IN GEBRUIK -> Applications
```

The filter `flag eq "IN GEBRUIK"` is the edge. It guarantees the child's
record set equals exactly the parent's green branch, so composing the two is
valid (the conditioning is explicit, nothing was marginalized away). The
margins warning above only applied to an *unfiltered* child.

This relation is **verifiable**, not a guess:

- the child must filter on the same source as the parent's dimension
  (`child.filter.source == parent.dimension.source`), and
- the child's filter value must equal the branch it is attached to
  (`child.filter.value == branch`).

A link that fails either check is rejected at author time, so a composite can
never silently chart the wrong subset.

## Object model

A composite is a small directed graph: a parent object plus a set of edges,
each mapping one parent branch value to a child object that filters to that
branch. Unmapped branches are leaves (drawn solid). Data still comes from the
existing objects evaluating themselves on their own DSL; the composite adds
only the wiring.

```yaml
statistics:
  - name: in-use-by-app
    label: "In Gebruik, drilled by application"
    composite:
      parent: in-use            # references an existing object by name
      edges:
        - branch: "IN GEBRUIK"
          child: applications   # references an existing object by name
        # NIET IN GEBRUIK has no edge -> solid leaf
```

Children referencing children is the natural recursion (deeper sunburst
rings), but the first cut can cap at depth 2 (parent + one child level) and
lift the cap once the renderer and the validator hold up.

### Carry-records rule

The implementation invariant: a hop passes a record selection (a predicate /
the filter), never a previously computed grid. The engine already has what it
needs for this. `AggregateRaw` carries form identity (`StatRawRow.Form`, added
for `records()`), so "which records are in this branch" is available to hand to
the next hop, and `records()` already counts distinct forms per category.

## Open choice: the leaf units do not partition

The relation is correct, but the green ring still will not partition cleanly.
The 71 in-use records each list several applications, so the application
children counted with `records()` overlap and sum past 71. Three honest
options, to decide before building the renderer:

1. **Overlapping drill panel.** Render the children as their own bar/pie under
   the branch, not as a strict subdivision of the parent arc. Honest, no data
   model change, but it is a drill-in, not a true sunburst ring.
2. **Primary application per record.** Add a single "primary application"
   field per record so the breakdown partitions, with a "rest / none" bucket
   for records that have no primary. True sunburst, but it needs a data-model
   decision and author effort.
3. **Normalize.** Scale the overlapping children to fill the arc. Rejected:
   it fabricates proportions and reintroduces the "lying chart" problem.

Recommendation: ship option 1 first (it is correct and needs no schema
change), offer option 2 later for templates that genuinely have a primary
application.

## Renderer

New chart shape: nested / sunburst (or, for option 1, a parent ring plus a
per-branch drill panel). It consumes the composite graph: parent ring from the
parent object's rank-1 grid, child ring(s)/panel(s) from each edge's child
object, colored by the same facet/palette rules already in `grid.ts`. Siblings
without an edge stay solid leaves. The plugin's shape picker (`refresh()` in
formstats) would gain "sunburst" when the selected object is a composite.

## Scope

This is a sizable addition, not a small change:

- engine/types: a `composite` object kind (parent + edges) alongside the DSL
  string kind; author-time validator for the filter-to-branch relation.
- builder: a way to author edges (pick a parent branch, attach a child object).
- renderer: the nested/sunburst (or drill-panel) component.
- plugin: expose the new shape.

No new aggregation math: composites reuse the existing per-object evaluation.
The new surface is structure (edges), validation (the relation), and a
hierarchical renderer.

## Decisions to confirm

- Leaf-units option (1 / 2 / 3 above). Recommend 1 first.
- Recursion depth: cap at 2 initially, or allow arbitrary depth.
- Where the composite kind lives: extend the `statistics:` entry shape with a
  `composite:` block (above), keeping `dsl:` and `composite:` mutually
  exclusive per object.
