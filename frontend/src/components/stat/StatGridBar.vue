<script setup lang="ts">
import { computed } from "vue";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { type Grid, denseRank1, densePct, facetColorToken, fmtNum, byValueDesc } from "./grid";

// Rank-1 grid as a horizontal bar chart of one measure across axis 0's
// labels. Percentages are shown against grid.total when the measure is a
// count. When the axis is a facet, each bar takes the facet option's
// authored color.
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

const PAD_RIGHT = 60;
const PAD_TOP = 12;
const BAR_HEIGHT = 18;
const BAR_GAP = 10;

const isCount = computed(() => (props.grid.measures[props.measureIndex] ?? "") === "count");

const view = computed(() => {
  const labels = props.grid.axes[0]?.labels ?? [];
  const axisSource = props.grid.axes[0]?.source ?? "";
  const values = denseRank1(props.grid, props.measureIndex);
  const pcts = densePct(props.grid, props.measureIndex);
  if (labels.length === 0) return null;

  // Left gutter sizes to the longest label so long facet-option names
  // aren't clipped at the viewBox edge. ~8 units/char + margin is
  // generous enough for the 12px label font once the canvas caps the
  // horizontal stretch; capped so the bars still get room.
  const maxChars = Math.max(0, ...labels.map((l) => (l === "" ? 7 : l.length)));
  const padLeft = Math.min(340, Math.max(130, Math.round(maxChars * 8) + 24));

  const max = Math.max(1, ...values.map((v) => Math.abs(v)));
  const barAreaW = props.width - padLeft - PAD_RIGHT;

  // Bars read highest-first by the measure on screen. Display order is a
  // renderer concern (the dialog can switch the shown measure), so it is
  // sorted here rather than baked into the axis: top-N already ranks the
  // axis by the FIRST measure to pick the categories; this re-sorts by the
  // one actually being drawn. Stable, so equal values keep axis order.
  const rows = labels
    .map((raw, i) => {
      const value = values[i] ?? 0;
      // Share of the distribution, computed server-side; shown for count.
      const pct = isCount.value ? Math.round(pcts[i] ?? 0) : null;
      const token = facetColorToken(props.facets, axisSource, raw);
      return {
        value,
        width: (Math.abs(value) / max) * barAreaW,
        label: raw === "" ? "(unset)" : raw,
        text: fmtNum(value),
        pct,
        // Facet option color (currentColor + class) or the default .stat-bar fill.
        colorClass: token ? `expr-text-${token}` : "",
        fill: token ? "currentColor" : "",
      };
    })
    .sort(byValueDesc)
    .map((r, i) => ({ ...r, y: PAD_TOP + i * (BAR_HEIGHT + BAR_GAP) }));

  const height = PAD_TOP + rows.length * (BAR_HEIGHT + BAR_GAP) + 4;
  return { rows, padLeft, height };
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
        <text :x="view.padLeft - 4" :y="row.y + 13" text-anchor="end" class="stat-bar-label">{{ row.label }}</text>
        <rect
          :x="view.padLeft"
          :y="row.y"
          :width="row.width"
          :height="BAR_HEIGHT"
          :class="['stat-bar', row.colorClass]"
          :style="row.fill ? { fill: 'currentColor' } : undefined"
        />
        <text :x="view.padLeft + row.width + 6" :y="row.y + 13" class="stat-bar-value">
          {{ row.text }}<tspan v-if="row.pct !== null"> ({{ row.pct }}%)</tspan>
        </text>
      </g>
    </svg>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
