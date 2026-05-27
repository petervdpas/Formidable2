<script setup lang="ts">
import { computed } from "vue";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { type Grid, denseRank1, densePct, facetColorToken, fmtNum, fmtPct, byValueDesc, CHART_PALETTE } from "./grid";

// Rank-1 grid as a pie of one measure across axis 0's labels, with the
// legend drawn INSIDE the same <svg> (swatch + text) so the chart is a
// single self-contained SVG - it renders identically everywhere
// (browser, VS Code, Inkscape) and exports cleanly. Slices are
// proportional to each label's value over their sum (negative/zero
// dropped). When the axis is a facet, slices/swatches take the facet
// option's authored color; otherwise the neutral palette.
const props = withDefaults(
  defineProps<{ grid: Grid; facets?: Facet[]; measureIndex?: number; size?: number }>(),
  { measureIndex: 0, size: 200 },
);

const PAD = 22; // margin around the whole chart (pie + legend)
const GAP = 16; // between pie and legend
const ROW = 22; // legend row height
const SWATCH = 13;

const view = computed(() => {
  const labels = props.grid.axes[0]?.labels ?? [];
  const axisSource = props.grid.axes[0]?.source ?? "";
  const values = denseRank1(props.grid, props.measureIndex);
  const pcts = densePct(props.grid, props.measureIndex);
  const slices = labels
    .map((raw, i) => ({ raw, label: raw === "" ? "(unset)" : raw, value: values[i] ?? 0, pct: pcts[i] ?? 0 }))
    .filter((s) => s.value > 0)
    .sort(byValueDesc);
  const total = slices.reduce((a, s) => a + s.value, 0);
  if (total <= 0) return null;

  const pie = props.size;
  const r = pie / 2;

  // Legend text per slice + viewBox width sized to the longest line.
  const texts = slices.map(
    (s) => `${s.label} - ${fmtNum(s.value)} (${fmtPct(s.pct)}%)`,
  );
  const maxChars = Math.max(0, ...texts.map((t) => t.length));
  const legendW = SWATCH + 8 + Math.round(maxChars * 6.2);
  const W = Math.max(pie, legendW) + PAD * 2;
  const pieX = (W - pie) / 2;

  let angle = -Math.PI / 2; // 12 o'clock
  const arcs = slices.map((s, i) => {
    const frac = s.value / total;
    const start = angle;
    const end = angle + frac * Math.PI * 2;
    angle = end;
    const x1 = r + r * Math.cos(start);
    const y1 = r + r * Math.sin(start);
    const x2 = r + r * Math.cos(end);
    const y2 = r + r * Math.sin(end);
    const large = end - start > Math.PI ? 1 : 0;
    const d =
      frac >= 1
        ? `M ${r} ${0} A ${r} ${r} 0 1 1 ${r - 0.01} ${0} Z`
        : `M ${r} ${r} L ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} Z`;
    const token = facetColorToken(props.facets, axisSource, s.raw);
    return {
      d,
      colorClass: token ? `expr-text-${token}` : "",
      fill: token ? "" : CHART_PALETTE[i % CHART_PALETTE.length],
      text: texts[i],
    };
  });

  const legendY0 = PAD + pie + GAP;
  const legend = arcs.map((a, i) => ({
    y: legendY0 + i * ROW,
    colorClass: a.colorClass,
    fill: a.fill,
    text: a.text,
  }));
  const H = legendY0 + arcs.length * ROW + PAD;
  return { arcs, legend, pieX, W, H };
});
</script>

<template>
  <div class="stat-chart">
    <svg
      v-if="view"
      class="stat-svg stat-pie-svg"
      :viewBox="`0 0 ${view.W} ${view.H}`"
      :style="{ width: `${view.W}px`, maxWidth: '100%', height: 'auto' }"
    >
      <g :transform="`translate(${view.pieX}, ${PAD})`">
        <path
          v-for="(a, i) in view.arcs"
          :key="`slice-${i}`"
          :d="a.d"
          :class="['stat-pie-slice', a.colorClass]"
          :style="{ fill: a.colorClass ? 'currentColor' : a.fill }"
          fill-opacity="0.82"
        />
      </g>
      <g v-for="(l, i) in view.legend" :key="`leg-${i}`">
        <rect
          :x="PAD"
          :y="l.y"
          :width="SWATCH"
          :height="SWATCH"
          rx="2"
          :class="l.colorClass"
          :style="{ fill: l.colorClass ? 'currentColor' : l.fill }"
        />
        <text :x="PAD + SWATCH + 6" :y="l.y + SWATCH - 2" class="stat-bar-label">{{ l.text }}</text>
      </g>
    </svg>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
