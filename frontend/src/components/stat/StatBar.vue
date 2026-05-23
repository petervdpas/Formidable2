<script setup lang="ts">
import { computed } from "vue";
import type { StatResult } from "./types";

// Horizontal bar chart for a single-series distribution. categories
// are the row labels; series[0].values are the counts. When total is
// known, each bar shows its share as a percentage. Mirrors the monitor
// module's MonitorBars layout so it themes for free.
const props = withDefaults(
  defineProps<{
    result: StatResult | null;
    width?: number;
    height?: number;
  }>(),
  { width: 520, height: 200 },
);

const PAD_LEFT = 120;
const PAD_RIGHT = 52;
const PAD_TOP = 12;
const BAR_HEIGHT = 18;
const BAR_GAP = 10;

const view = computed(() => {
  const cats = props.result?.categories ?? [];
  const values = props.result?.series?.[0]?.values ?? [];
  if (cats.length === 0) return null;

  const max = Math.max(1, ...values);
  const barAreaW = props.width - PAD_LEFT - PAD_RIGHT;
  const total = props.result?.total ?? 0;

  const rows = cats.map((label, i) => {
    const value = values[i] ?? 0;
    const pct = total > 0 ? Math.round((value / total) * 100) : null;
    return {
      y: PAD_TOP + i * (BAR_HEIGHT + BAR_GAP),
      width: (value / max) * barAreaW,
      label: label === "" ? "(unset)" : label,
      value,
      pct,
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
        <text
          :x="PAD_LEFT - 4"
          :y="row.y + 13"
          text-anchor="end"
          class="stat-bar-label"
        >{{ row.label }}</text>
        <rect
          :x="PAD_LEFT"
          :y="row.y"
          :width="row.width"
          :height="BAR_HEIGHT"
          class="stat-bar"
        />
        <text
          :x="PAD_LEFT + row.width + 6"
          :y="row.y + 13"
          class="stat-bar-value"
        >{{ row.value }}<tspan v-if="row.pct !== null"> ({{ row.pct }}%)</tspan></text>
      </g>
    </svg>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
