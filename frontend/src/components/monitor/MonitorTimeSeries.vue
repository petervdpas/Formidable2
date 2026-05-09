<script setup lang="ts">
import { computed } from "vue";
import type { Result } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/monitor/models";

const props = withDefaults(
  defineProps<{
    result: Result | null;
    width?: number;
    height?: number;
  }>(),
  { width: 520, height: 200 },
);

// Layout constants. SVG is rendered in a viewBox so the parent
// container can resize via CSS without losing crispness.
const PAD_LEFT = 36;
const PAD_RIGHT = 12;
const PAD_TOP = 8;
const PAD_BOTTOM = 22;

// Simple stable palette. Series get assigned colors in their order
// of appearance; backend already returns Series sorted by joined Key
// so colors don't shuffle across refreshes.
const PALETTE = [
  "var(--color-accent, #5b9cff)",
  "var(--color-accent-2, #f08c5a)",
  "var(--color-success, #5fc37a)",
  "var(--color-warn, #d4a64a)",
  "var(--color-danger, #d96666)",
  "var(--color-text-muted, #888)",
];

interface Stack {
  ts: number; // unix ms
  cum: number[]; // cumulative top per series index, parallel to series
}

const view = computed(() => {
  const series = props.result?.series ?? [];
  if (series.length === 0) return null;

  // Collect all unique timestamps across series (union, sorted).
  const tsSet = new Set<number>();
  for (const s of series) {
    for (const p of s.points ?? []) {
      const t = p.ts ? new Date(p.ts).getTime() : 0;
      if (!Number.isNaN(t)) tsSet.add(t);
    }
  }
  if (tsSet.size === 0) return null;
  const allTs = Array.from(tsSet).sort((a, b) => a - b);

  // Per-series, per-timestamp value lookup. 0 when absent.
  const valueAt = (sIdx: number, t: number): number => {
    const s = series[sIdx];
    for (const p of s.points ?? []) {
      const tp = p.ts ? new Date(p.ts).getTime() : 0;
      if (tp === t) return p.value ?? 0;
    }
    return 0;
  };

  // Build stacks: at each ts, cum[i] = sum(values[0..=i]).
  const stacks: Stack[] = allTs.map((ts) => {
    const cum: number[] = [];
    let running = 0;
    for (let i = 0; i < series.length; i++) {
      running += valueAt(i, ts);
      cum.push(running);
    }
    return { ts, cum };
  });

  const yMax = stacks.reduce((m, s) => Math.max(m, s.cum[s.cum.length - 1]), 0) || 1;
  const xMin = allTs[0];
  const xMax = allTs[allTs.length - 1];
  const xSpan = Math.max(1, xMax - xMin);

  const xToPx = (t: number): number =>
    PAD_LEFT + ((t - xMin) / xSpan) * (props.width - PAD_LEFT - PAD_RIGHT);
  const yToPx = (v: number): number =>
    props.height - PAD_BOTTOM - (v / yMax) * (props.height - PAD_TOP - PAD_BOTTOM);

  // Build one path per series. Each fills the band between cum[i-1]
  // and cum[i] across all timestamps, drawn from left → right along
  // the top (cum[i]) and back right → left along the bottom (cum[i-1]).
  const paths: { d: string; color: string; label: string }[] = [];
  for (let i = 0; i < series.length; i++) {
    const top = stacks.map((s) => `${xToPx(s.ts)},${yToPx(s.cum[i])}`);
    const bot = stacks
      .map((s) => `${xToPx(s.ts)},${yToPx(i === 0 ? 0 : s.cum[i - 1])}`)
      .reverse();
    const d = `M ${top.join(" L ")} L ${bot.join(" L ")} Z`;
    const label = formatSeriesKey(series[i].key);
    paths.push({ d, color: PALETTE[i % PALETTE.length], label });
  }

  // X-axis tick labels: ~5 evenly-spaced timestamps.
  const tickCount = Math.min(5, allTs.length);
  const ticks: { x: number; label: string }[] = [];
  for (let i = 0; i < tickCount; i++) {
    const t = allTs[Math.floor((i * (allTs.length - 1)) / Math.max(1, tickCount - 1))];
    ticks.push({ x: xToPx(t), label: formatHour(t) });
  }

  // Y-axis ticks: 0, mid, max.
  const yTicks = [
    { y: yToPx(0), label: "0" },
    { y: yToPx(yMax / 2), label: formatNum(yMax / 2) },
    { y: yToPx(yMax), label: formatNum(yMax) },
  ];

  return { paths, ticks, yTicks };
});

function formatSeriesKey(key: { [k: string]: string | undefined }): string {
  const parts = Object.entries(key)
    .filter(([_, v]) => v !== undefined && v !== "")
    .map(([k, v]) => `${k}=${v}`);
  return parts.length === 0 ? "all" : parts.join(", ");
}

function formatHour(ms: number): string {
  const d = new Date(ms);
  const hh = String(d.getHours()).padStart(2, "0");
  return `${hh}:00`;
}

function formatNum(v: number): string {
  if (v < 10) return v.toFixed(1).replace(/\.0$/, "");
  return String(Math.round(v));
}
</script>

<template>
  <div class="monitor-chart">
    <svg
      v-if="view"
      class="monitor-svg"
      :viewBox="`0 0 ${props.width} ${props.height}`"
      preserveAspectRatio="none"
    >
      <!-- Y-axis grid + labels -->
      <g class="monitor-axis">
        <line
          v-for="(t, i) in view.yTicks"
          :key="`y-${i}`"
          :x1="36"
          :x2="props.width - 12"
          :y1="t.y"
          :y2="t.y"
          class="monitor-grid"
        />
        <text
          v-for="(t, i) in view.yTicks"
          :key="`yl-${i}`"
          :x="32"
          :y="t.y + 3"
          text-anchor="end"
          class="monitor-axis-label"
        >{{ t.label }}</text>
      </g>

      <!-- X-axis labels -->
      <g class="monitor-axis">
        <text
          v-for="(t, i) in view.ticks"
          :key="`x-${i}`"
          :x="t.x"
          :y="props.height - 6"
          text-anchor="middle"
          class="monitor-axis-label"
        >{{ t.label }}</text>
      </g>

      <!-- Stacked area paths -->
      <g class="monitor-series">
        <path
          v-for="(p, i) in view.paths"
          :key="`p-${i}`"
          :d="p.d"
          :fill="p.color"
          fill-opacity="0.65"
          stroke="none"
        />
      </g>
    </svg>
    <p v-else class="monitor-empty">No data in range.</p>

    <ul v-if="view" class="monitor-legend">
      <li v-for="(p, i) in view.paths" :key="`leg-${i}`" class="legend-row">
        <span class="legend-swatch" :style="{ background: p.color }" />
        <span class="legend-label">{{ p.label }}</span>
      </li>
    </ul>
  </div>
</template>
