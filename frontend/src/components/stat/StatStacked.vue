<script setup lang="ts">
import { computed } from "vue";
import { CHART_PALETTE, type StatResult } from "./types";

// Vertical stacked-bar chart for a cross-tab. categories are the
// x-axis columns (the A-facet options); each series is one B-facet
// option, stacked within each column. Legend maps colors to series.
const props = withDefaults(
  defineProps<{
    result: StatResult | null;
    width?: number;
    height?: number;
  }>(),
  { width: 520, height: 240 },
);

const PAD_LEFT = 36;
const PAD_RIGHT = 12;
const PAD_TOP = 12;
const PAD_BOTTOM = 28;

const view = computed(() => {
  const cats = props.result?.categories ?? [];
  const series = props.result?.series ?? [];
  if (cats.length === 0 || series.length === 0) return null;

  // Column totals = sum across series for each category.
  const colTotals = cats.map((_, ci) =>
    series.reduce((sum, s) => sum + (s.values[ci] ?? 0), 0),
  );
  const yMax = Math.max(1, ...colTotals);

  const plotW = props.width - PAD_LEFT - PAD_RIGHT;
  const plotH = props.height - PAD_TOP - PAD_BOTTOM;
  const slot = plotW / cats.length;
  const barW = Math.min(48, slot * 0.6);

  const yToPx = (v: number) => PAD_TOP + plotH - (v / yMax) * plotH;

  const columns = cats.map((label, ci) => {
    const x = PAD_LEFT + ci * slot + (slot - barW) / 2;
    let running = 0;
    const segments = series.map((s, si) => {
      const value = s.values[ci] ?? 0;
      const y0 = running;
      running += value;
      return {
        x,
        y: yToPx(running),
        width: barW,
        height: yToPx(y0) - yToPx(running),
        color: CHART_PALETTE[si % CHART_PALETTE.length],
        value,
      };
    });
    return { label: label === "" ? "(unset)" : label, x: x + barW / 2, segments };
  });

  const legend = series.map((s, si) => ({
    label: s.name === "" ? "(unset)" : s.name,
    color: CHART_PALETTE[si % CHART_PALETTE.length],
  }));

  const yTicks = [
    { y: yToPx(0), label: "0" },
    { y: yToPx(yMax / 2), label: String(Math.round(yMax / 2)) },
    { y: yToPx(yMax), label: String(yMax) },
  ];

  return { columns, legend, yTicks };
});
</script>

<template>
  <div class="stat-chart">
    <svg
      v-if="view"
      class="stat-svg"
      :viewBox="`0 0 ${props.width} ${props.height}`"
      preserveAspectRatio="none"
    >
      <g>
        <line
          v-for="(t, i) in view.yTicks"
          :key="`y-${i}`"
          :x1="PAD_LEFT"
          :x2="props.width - PAD_RIGHT"
          :y1="t.y"
          :y2="t.y"
          class="stat-grid"
        />
        <text
          v-for="(t, i) in view.yTicks"
          :key="`yl-${i}`"
          :x="PAD_LEFT - 4"
          :y="t.y + 3"
          text-anchor="end"
          class="stat-axis-label"
        >{{ t.label }}</text>
      </g>

      <g v-for="(col, ci) in view.columns" :key="`col-${ci}`">
        <rect
          v-for="(seg, si) in col.segments"
          :key="`seg-${ci}-${si}`"
          :x="seg.x"
          :y="seg.y"
          :width="seg.width"
          :height="seg.height"
          :fill="seg.color"
          fill-opacity="0.78"
        />
        <text
          :x="col.x"
          :y="props.height - 10"
          text-anchor="middle"
          class="stat-axis-label"
        >{{ col.label }}</text>
      </g>
    </svg>
    <p v-else class="stat-empty">No data.</p>

    <ul v-if="view" class="stat-legend">
      <li v-for="(l, i) in view.legend" :key="`leg-${i}`" class="stat-legend-row">
        <span class="stat-legend-swatch" :style="{ background: l.color }" />
        <span class="stat-legend-label">{{ l.label }}</span>
      </li>
    </ul>
  </div>
</template>
