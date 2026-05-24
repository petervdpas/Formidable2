// Local structural mirror of the Go stat.Grid (rank-N values grid). Kept
// local (like StatResult in ./types) so the Grid chart components don't
// hard-depend on the generated bindings; the generated type is
// structurally assignable. The rank is axes.length: 0 = scalar cards,
// 1 = bar/pie, 2 = heatmap.

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

/** Compact number formatting: integers stay integers, fractions show up
 *  to two decimals with trailing zeros trimmed. */
export function fmtNum(v: number): string {
  if (Number.isInteger(v)) return String(v);
  return v.toFixed(2).replace(/\.?0+$/, "");
}
