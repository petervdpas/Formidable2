<script setup lang="ts">
import { computed } from "vue";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { type Grid, denseRank1, facetColorToken, fmtNum } from "./grid";

// Rank-1 grid as a horizontal bar chart of one measure across axis 0's
// labels. Mirrors the StatBar layout (it themes for free). Percentages
// are shown against grid.total when the measure is a count. When the axis
// is a facet, each bar takes the facet option's authored color.
const props = withDefaults(
  defineProps<{
    grid: Grid;
    facets?: Facet[];
    measureIndex?: number;
    width?: number;
    height?: number;
  }>(),
  { measureIndex: 0, width: 520, height: 200 },
);

const PAD_LEFT = 120;
const PAD_RIGHT = 60;
const PAD_TOP = 12;
const BAR_HEIGHT = 18;
const BAR_GAP = 10;

const isCount = computed(() => (props.grid.measures[props.measureIndex] ?? "") === "count");

const view = computed(() => {
  const labels = props.grid.axes[0]?.labels ?? [];
  const axisSource = props.grid.axes[0]?.source ?? "";
  const values = denseRank1(props.grid, props.measureIndex);
  if (labels.length === 0) return null;

  const max = Math.max(1, ...values.map((v) => Math.abs(v)));
  const barAreaW = props.width - PAD_LEFT - PAD_RIGHT;
  const total = props.grid.total ?? 0;

  const rows = labels.map((raw, i) => {
    const value = values[i] ?? 0;
    const pct = isCount.value && total > 0 ? Math.round((value / total) * 100) : null;
    const token = facetColorToken(props.facets, axisSource, raw);
    return {
      y: PAD_TOP + i * (BAR_HEIGHT + BAR_GAP),
      width: (Math.abs(value) / max) * barAreaW,
      label: raw === "" ? "(unset)" : raw,
      text: fmtNum(value),
      pct,
      // Facet option color (currentColor + class) or the default .stat-bar fill.
      colorClass: token ? `expr-text-${token}` : "",
      fill: token ? "currentColor" : "",
    };
  });

  const minHeight = PAD_TOP + rows.length * (BAR_HEIGHT + BAR_GAP) + 4;
  return { rows, height: Math.max(props.height, minHeight) };
});
</script>

<template>
  <div class="stat-chart">
    <svg
      v-if="view"
      class="stat-svg"
      :viewBox="`0 0 ${props.width} ${view.height}`"
      preserveAspectRatio="none"
    >
      <g v-for="(row, i) in view.rows" :key="`bar-${i}`">
        <text :x="PAD_LEFT - 4" :y="row.y + 13" text-anchor="end" class="stat-bar-label">{{ row.label }}</text>
        <rect
          :x="PAD_LEFT"
          :y="row.y"
          :width="row.width"
          :height="BAR_HEIGHT"
          :class="['stat-bar', row.colorClass]"
          :style="row.fill ? { fill: 'currentColor' } : undefined"
        />
        <text :x="PAD_LEFT + row.width + 6" :y="row.y + 13" class="stat-bar-value">
          {{ row.text }}<tspan v-if="row.pct !== null"> ({{ row.pct }}%)</tspan>
        </text>
      </g>
    </svg>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
