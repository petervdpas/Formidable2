<script setup lang="ts">
import { computed } from "vue";
import { type Grid, denseRank1, fmtNum } from "./grid";
import { CHART_PALETTE } from "./types";

// Rank-1 grid as a pie of one measure across axis 0's labels. Slices are
// proportional to each label's value over their sum (negative/zero values
// are dropped - a pie can't show them). Same data a bar would use; the
// renderer is the consumer's choice, not the statistic's.
const props = withDefaults(
  defineProps<{ grid: Grid; measureIndex?: number; size?: number }>(),
  { measureIndex: 0, size: 220 },
);

const view = computed(() => {
  const labels = props.grid.axes[0]?.labels ?? [];
  const values = denseRank1(props.grid, props.measureIndex);
  const slices = labels
    .map((label, i) => ({ label: label === "" ? "(unset)" : label, value: values[i] ?? 0 }))
    .filter((s) => s.value > 0);
  const total = slices.reduce((a, s) => a + s.value, 0);
  if (total <= 0) return null;

  const r = props.size / 2;
  const cx = r;
  const cy = r;
  let angle = -Math.PI / 2; // start at 12 o'clock
  const arcs = slices.map((s, i) => {
    const frac = s.value / total;
    const start = angle;
    const end = angle + frac * Math.PI * 2;
    angle = end;
    const x1 = cx + r * Math.cos(start);
    const y1 = cy + r * Math.sin(start);
    const x2 = cx + r * Math.cos(end);
    const y2 = cy + r * Math.sin(end);
    const large = end - start > Math.PI ? 1 : 0;
    // A full single slice (frac===1) can't be drawn as an arc (start==end);
    // render it as a full circle path instead.
    const d =
      frac >= 1
        ? `M ${cx} ${cy - r} A ${r} ${r} 0 1 1 ${cx - 0.01} ${cy - r} Z`
        : `M ${cx} ${cy} L ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} Z`;
    return {
      d,
      color: CHART_PALETTE[i % CHART_PALETTE.length],
      label: s.label,
      value: fmtNum(s.value),
      pct: Math.round(frac * 100),
    };
  });
  return { arcs, size: props.size };
});
</script>

<template>
  <div class="stat-chart stat-pie-wrap">
    <template v-if="view">
      <svg
        class="stat-svg stat-pie-svg"
        :viewBox="`0 0 ${view.size} ${view.size}`"
        :style="{ width: `${view.size}px`, height: `${view.size}px` }"
      >
        <path
          v-for="(a, i) in view.arcs"
          :key="`slice-${i}`"
          :d="a.d"
          :fill="a.color"
          fill-opacity="0.82"
          class="stat-pie-slice"
        />
      </svg>
      <ul class="stat-legend">
        <li v-for="(a, i) in view.arcs" :key="`leg-${i}`" class="stat-legend-row">
          <span class="stat-legend-swatch" :style="{ background: a.color }" />
          <span class="stat-legend-label">{{ a.label }} - {{ a.value }} ({{ a.pct }}%)</span>
        </li>
      </ul>
    </template>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
