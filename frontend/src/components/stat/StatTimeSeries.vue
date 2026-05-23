<script setup lang="ts">
import { computed } from "vue";
import type { StatResult } from "./types";

// Line chart for a date timeseries. categories are the period labels
// (year / month / day, already chronologically ordered by the backend);
// series[0].values are the per-period counts. Drawn as a polyline with
// point markers and a light area fill under it.
const props = withDefaults(
  defineProps<{
    result: StatResult | null;
    width?: number;
    height?: number;
  }>(),
  { width: 520, height: 220 },
);

const PAD_LEFT = 36;
const PAD_RIGHT = 12;
const PAD_TOP = 12;
const PAD_BOTTOM = 28;

const view = computed(() => {
  const cats = props.result?.categories ?? [];
  const values = props.result?.series?.[0]?.values ?? [];
  if (cats.length === 0) return null;

  const yMax = Math.max(1, ...values);
  const plotW = props.width - PAD_LEFT - PAD_RIGHT;
  const plotH = props.height - PAD_TOP - PAD_BOTTOM;

  const xAt = (i: number) =>
    cats.length === 1
      ? PAD_LEFT + plotW / 2
      : PAD_LEFT + (i / (cats.length - 1)) * plotW;
  const yToPx = (v: number) => PAD_TOP + plotH - (v / yMax) * plotH;

  const points = cats.map((label, i) => ({
    x: xAt(i),
    y: yToPx(values[i] ?? 0),
    label,
    value: values[i] ?? 0,
  }));

  const line = points.map((p) => `${p.x},${p.y}`).join(" ");
  const baseY = yToPx(0);
  const area =
    `${points[0].x},${baseY} ` +
    points.map((p) => `${p.x},${p.y}`).join(" ") +
    ` ${points[points.length - 1].x},${baseY}`;

  // Thin the x labels when crowded so they don't overlap.
  const step = Math.ceil(cats.length / 8);
  const ticks = points
    .map((p, i) => ({ ...p, show: i % step === 0 || i === points.length - 1 }))
    .filter((p) => p.show);

  const yTicks = [
    { y: yToPx(0), label: "0" },
    { y: yToPx(yMax / 2), label: String(Math.round(yMax / 2)) },
    { y: yToPx(yMax), label: String(yMax) },
  ];

  return { points, line, area, ticks, yTicks };
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

      <polygon :points="view.area" class="stat-area" />
      <polyline :points="view.line" class="stat-line" />
      <circle
        v-for="(p, i) in view.points"
        :key="`pt-${i}`"
        :cx="p.x"
        :cy="p.y"
        r="3"
        class="stat-point"
      />

      <text
        v-for="(t, i) in view.ticks"
        :key="`x-${i}`"
        :x="t.x"
        :y="props.height - 10"
        text-anchor="middle"
        class="stat-axis-label"
      >{{ t.label }}</text>
    </svg>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
