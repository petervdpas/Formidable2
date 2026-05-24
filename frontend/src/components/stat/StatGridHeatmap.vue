<script setup lang="ts">
import { computed } from "vue";
import { type Grid, denseRank2, fmtNum } from "./grid";

// Rank-2 grid as a heatmap of one measure: axis 0 = rows, axis 1 = cols.
// Cell shade scales with the value over the matrix max. The corner names
// the two source axes.
const props = withDefaults(
  defineProps<{ grid: Grid; measureIndex?: number }>(),
  { measureIndex: 0 },
);

const view = computed(() => {
  const rowLabels = props.grid.axes[0]?.labels ?? [];
  const colLabels = props.grid.axes[1]?.labels ?? [];
  if (rowLabels.length === 0 || colLabels.length === 0) return null;
  const matrix = denseRank2(props.grid, props.measureIndex);
  let max = 0;
  for (const r of matrix) for (const v of r) max = Math.max(max, Math.abs(v));
  const cells = matrix.map((r) =>
    r.map((v) => ({
      value: v,
      text: v === 0 ? "" : fmtNum(v),
      // 0.08 floor so a populated-but-tiny cell still reads as filled.
      alpha: max > 0 && v !== 0 ? 0.12 + 0.78 * (Math.abs(v) / max) : 0,
    })),
  );
  return {
    rowLabels: rowLabels.map((l) => (l === "" ? "(unset)" : l)),
    colLabels: colLabels.map((l) => (l === "" ? "(unset)" : l)),
    cells,
    cols: colLabels.length,
    rowSource: props.grid.axes[0]?.source ?? "",
    colSource: props.grid.axes[1]?.source ?? "",
  };
});
</script>

<template>
  <div v-if="view" class="stat-heatmap" :style="{ '--heatmap-cols': view.cols }">
    <div class="stat-heatmap-corner">{{ view.rowSource }} \ {{ view.colSource }}</div>
    <div v-for="(c, ci) in view.colLabels" :key="`c${ci}`" class="stat-heatmap-col">{{ c }}</div>
    <template v-for="(row, ri) in view.cells" :key="`r${ri}`">
      <div class="stat-heatmap-rowlabel">{{ view.rowLabels[ri] }}</div>
      <div
        v-for="(cell, ci) in row"
        :key="`r${ri}c${ci}`"
        class="stat-heatmap-cell"
        :style="{ background: `rgba(var(--stat-heat-rgb, 91, 156, 255), ${cell.alpha})` }"
      >{{ cell.text }}</div>
    </template>
  </div>
  <p v-else class="stat-empty">No data.</p>
</template>
