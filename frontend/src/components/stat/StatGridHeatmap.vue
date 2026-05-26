<script setup lang="ts">
import { computed } from "vue";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { type Grid, denseRank2, denseRank2Pct, fmtNum } from "./grid";

// Rank-2 grid as a heatmap of one measure: axis 0 = rows, axis 1 = cols.
// Drawn as a single SVG (cell rects shaded by value over the matrix max,
// rotated column headers, row labels, corner naming the two axes) so the
// chart is self-contained and exports like the others. facets is
// accepted for a uniform dispatch signature but unused (a cell spans two
// facets, so per-option color wouldn't be meaningful).
const props = withDefaults(
  defineProps<{ grid: Grid; facets?: Facet[]; measureIndex?: number }>(),
  { measureIndex: 0 },
);

const PAD = 22;
const CELL_W = 44;
const CELL_H = 36; // two lines: the value, then its share of the matrix

function unset(l: string): string {
  return l === "" ? "(unset)" : l;
}
function trunc(s: string, n: number): string {
  return s.length > n ? `${s.slice(0, n - 1)}…` : s;
}

const view = computed(() => {
  const rowLabels = (props.grid.axes[0]?.labels ?? []).map(unset).map((l) => trunc(l, 24));
  const colLabels = (props.grid.axes[1]?.labels ?? []).map(unset).map((l) => trunc(l, 22));
  if (rowLabels.length === 0 || colLabels.length === 0) return null;

  const matrix = denseRank2(props.grid, props.measureIndex);
  const pctMatrix = denseRank2Pct(props.grid, props.measureIndex);
  let max = 0;
  for (const r of matrix) for (const v of r) max = Math.max(max, Math.abs(v));

  const maxRowChars = Math.max(0, ...rowLabels.map((l) => l.length));
  const maxColChars = Math.max(0, ...colLabels.map((l) => l.length));
  const rowLabelW = Math.min(200, Math.max(60, Math.round(maxRowChars * 6.5) + 12));
  const colHeaderH = Math.min(180, Math.max(44, Math.round(maxColChars * 6.5) + 12));

  const gridX = PAD + rowLabelW;
  const gridY = PAD + colHeaderH;
  const W = gridX + colLabels.length * CELL_W + PAD;
  const H = gridY + rowLabels.length * CELL_H + PAD;

  const cells = matrix.map((r, ri) =>
    r.map((v, ci) => ({
      x: gridX + ci * CELL_W,
      y: gridY + ri * CELL_H,
      // 0.12 floor so a populated-but-tiny cell still reads as filled.
      alpha: max > 0 && v !== 0 ? 0.12 + 0.78 * (Math.abs(v) / max) : 0,
      text: v === 0 ? "" : fmtNum(v),
      // Server-computed share of the matrix total (% of all cells).
      pct: v === 0 ? "" : `${Math.round(pctMatrix[ri][ci])}%`,
    })),
  );
  const cols = colLabels.map((label, ci) => ({ label, cx: gridX + ci * CELL_W + CELL_W / 2 }));
  const rows = rowLabels.map((label, ri) => ({ label, cy: gridY + ri * CELL_H + CELL_H / 2 }));

  return {
    W,
    H,
    gridY,
    gridX,
    cells,
    cols,
    rows,
    corner: `${props.grid.axes[0]?.source ?? ""} \\ ${props.grid.axes[1]?.source ?? ""}`,
  };
});
</script>

<template>
  <div class="stat-chart">
    <svg
      v-if="view"
      class="stat-svg"
      :viewBox="`0 0 ${view.W} ${view.H}`"
      :style="{ width: `${view.W}px`, maxWidth: '100%', height: 'auto' }"
    >
      <text :x="PAD" :y="view.gridY - 6" class="stat-svg-corner">{{ view.corner }}</text>
      <text
        v-for="(c, ci) in view.cols"
        :key="`col-${ci}`"
        :x="c.cx"
        :y="view.gridY - 6"
        text-anchor="start"
        class="stat-bar-label"
        :transform="`rotate(-90, ${c.cx}, ${view.gridY - 6})`"
      >{{ c.label }}</text>
      <text
        v-for="(r, ri) in view.rows"
        :key="`row-${ri}`"
        :x="view.gridX - 6"
        :y="r.cy + 4"
        text-anchor="end"
        class="stat-bar-label"
      >{{ r.label }}</text>
      <template v-for="(row, ri) in view.cells" :key="`r-${ri}`">
        <g v-for="(cell, ci) in row" :key="`r-${ri}-c-${ci}`">
          <rect
            :x="cell.x"
            :y="cell.y"
            :width="CELL_W"
            :height="CELL_H"
            class="stat-svg-heatcell"
            :fill-opacity="cell.alpha"
          />
          <text
            v-if="cell.text"
            :x="cell.x + CELL_W / 2"
            :y="cell.y + CELL_H / 2 - 1"
            text-anchor="middle"
            class="stat-bar-value"
          >{{ cell.text }}</text>
          <text
            v-if="cell.pct"
            :x="cell.x + CELL_W / 2"
            :y="cell.y + CELL_H / 2 + 11"
            text-anchor="middle"
            class="stat-heat-pct"
          >{{ cell.pct }}</text>
        </g>
      </template>
    </svg>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
