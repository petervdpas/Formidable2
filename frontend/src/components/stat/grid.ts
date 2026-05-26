// Local structural mirror of the Go stat.Grid (rank-N values grid). Kept
// local so the Grid chart components don't hard-depend on the generated
// bindings; the generated type is structurally assignable. The rank is
// axes.length: 0 = scalar cards, 1 = bar/pie, 2 = heatmap.

import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Neutral fallback palette for non-facet categories (a facet axis uses
// the facets' authored option colors via facetColorToken instead).
// Ordered most-distinct-first so a top-N chart (N up to 20) keeps adjacent
// categories separable: the first ten are well-spaced hues, the tail fills
// out the rest. The first five stay theme-tied for in-app harmony.
export const CHART_PALETTE = [
  "var(--color-accent, #5b9cff)", // blue
  "var(--color-accent-2, #f08c5a)", // orange
  "var(--color-success, #5fc37a)", // green
  "var(--color-warn, #d4a64a)", // amber
  "var(--color-danger, #d96666)", // red
  "#b388eb", // violet
  "#4dd0c1", // teal
  "#e879b9", // pink
  "#c0ca33", // lime
  "var(--color-text-muted, #8a93a6)", // slate grey
  "#a1887f", // taupe
  "#7e6bd4", // indigo
  "#e0935b", // ochre
  "#5aa9e6", // sky
  "#8bc34a", // leaf
  "#cf6679", // rose
];

export interface GridAxis {
  source: string;
  labels: string[];
}
export interface GridCell {
  coords: number[];
  values: number[];
}
export interface Grid {
  axes: GridAxis[];
  measures: string[];
  cells: GridCell[];
  total: number;
}

export function gridRank(g: Grid | null): number {
  return g?.axes?.length ?? 0;
}

/** Dense 1D vector of one measure's values aligned to axis 0's labels
 *  (sparse cells default to 0). */
export function denseRank1(g: Grid, measureIdx: number): number[] {
  const n = g.axes[0]?.labels.length ?? 0;
  const out = new Array<number>(n).fill(0);
  for (const c of g.cells) {
    if (c.coords.length === 1) out[c.coords[0]] = c.values[measureIdx] ?? 0;
  }
  return out;
}

/** Dense rows x cols matrix of one measure (axis0 = rows, axis1 = cols). */
export function denseRank2(g: Grid, measureIdx: number): number[][] {
  const rows = g.axes[0]?.labels.length ?? 0;
  const cols = g.axes[1]?.labels.length ?? 0;
  const out = Array.from({ length: rows }, () => new Array<number>(cols).fill(0));
  for (const c of g.cells) {
    if (c.coords.length === 2) out[c.coords[0]][c.coords[1]] = c.values[measureIdx] ?? 0;
  }
  return out;
}

/** Rank-0: one (measure label, value) pair per measure from the single cell. */
export function scalarValues(g: Grid): { label: string; value: number }[] {
  const cell = g.cells[0];
  return (g.measures ?? []).map((m, i) => ({ label: m, value: cell?.values[i] ?? 0 }));
}

/** When an axis is a facet, its category labels are facet-option labels,
 *  each with an authored color token. facetColorToken maps an axis source
 *  (the facet key) + a raw label to that token, or null when the axis is
 *  not a facet / the label has no option. Lets a 1D renderer paint
 *  categories in the same colors as the facet's pills. */
export function facetColorToken(
  facets: Facet[] | undefined,
  axisSource: string,
  rawLabel: string,
): string | null {
  const f = (facets ?? []).find((x) => x.key === axisSource);
  if (!f) return null;
  const o = (f.options ?? []).find((opt) => opt.label === rawLabel);
  return o?.color || null;
}

/** Compact number formatting: integers stay integers, fractions show up
 *  to two decimals with trailing zeros trimmed. */
export function fmtNum(v: number): string {
  if (Number.isInteger(v)) return String(v);
  return v.toFixed(2).replace(/\.?0+$/, "");
}
