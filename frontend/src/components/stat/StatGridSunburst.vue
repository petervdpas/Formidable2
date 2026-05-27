<script setup lang="ts">
import { computed } from "vue";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import {
  type CompositeGrid,
  type Grid,
  denseRank1,
  densePct,
  facetColorToken,
  byValueDesc,
  fmtNum,
  fmtPct,
  CHART_PALETTE,
} from "./grid";

// A composite (hop route) as a sunburst: the parent rank-1 grid is the inner
// pie, and each drilled branch's child grid fills that branch's arc as an
// outer ring. Branches with no child stay solid (inner slice only). The
// outer-ring segments are proportional to the child distribution WITHIN the
// branch (the child values do not partition the parent count, so the ring
// shows relative composition, not absolute record counts - see
// design/statistics-composite.md). Self-contained SVG (chart + legend) so it
// downloads cleanly, matching StatGridPie.
const props = withDefaults(
  defineProps<{
    composite: CompositeGrid;
    facets?: Facet[];
    size?: number;
    parentMeasureIndex?: number;
    childMeasureIndex?: number;
  }>(),
  { size: 240, parentMeasureIndex: 0, childMeasureIndex: 0 },
);

const PAD = 22;
const GAP = 16;
const ROW = 22;
const SWATCH = 13;

function polar(c: number, r: number, a: number): [number, number] {
  return [c + r * Math.cos(a), c + r * Math.sin(a)];
}
function pieSlice(c: number, r: number, a0: number, a1: number): string {
  if (a1 - a0 >= Math.PI * 2 - 1e-9) {
    return `M ${c} ${c - r} A ${r} ${r} 0 1 1 ${c - 0.01} ${c - r} Z`;
  }
  const [x0, y0] = polar(c, r, a0);
  const [x1, y1] = polar(c, r, a1);
  const large = a1 - a0 > Math.PI ? 1 : 0;
  return `M ${c} ${c} L ${x0} ${y0} A ${r} ${r} 0 ${large} 1 ${x1} ${y1} Z`;
}
function ringSeg(c: number, ri: number, ro: number, a0: number, a1: number): string {
  const [xi0, yi0] = polar(c, ri, a0);
  const [xo0, yo0] = polar(c, ro, a0);
  const [xi1, yi1] = polar(c, ri, a1);
  const [xo1, yo1] = polar(c, ro, a1);
  const large = a1 - a0 > Math.PI ? 1 : 0;
  return `M ${xi0} ${yi0} L ${xo0} ${yo0} A ${ro} ${ro} 0 ${large} 1 ${xo1} ${yo1} L ${xi1} ${yi1} A ${ri} ${ri} 0 ${large} 0 ${xi0} ${yi0} Z`;
}

interface Slice {
  raw: string;
  label: string;
  value: number;
  pct: number; // server-computed share of the measure total
}
function slicesOf(g: Grid, measureIdx: number): Slice[] {
  const labels = g.axes[0]?.labels ?? [];
  const values = denseRank1(g, measureIdx);
  const pcts = densePct(g, measureIdx);
  return labels
    .map((raw, i) => ({
      raw,
      label: raw === "" ? "(unset)" : raw,
      value: values[i] ?? 0,
      pct: pcts[i] ?? 0,
    }))
    .filter((s) => s.value > 0)
    .sort(byValueDesc);
}

interface Arc {
  d: string;
  colorClass: string;
  fill: string;
}
interface LegendRow {
  y: number;
  indent: boolean;
  header: boolean;
  colorClass: string;
  fill: string;
  text: string;
}

const view = computed(() => {
  const parent = props.composite.parent;
  if (!parent || !parent.axes?.[0]) return null;
  const parentSource = parent.axes[0].source ?? "";
  const parentSlices = slicesOf(parent, props.parentMeasureIndex);
  const parentTotal = parentSlices.reduce((a, s) => a + s.value, 0);
  if (parentTotal <= 0) return null;

  // child grid per branch value, from the backend's branch list.
  const childByBranch = new Map<string, Grid | null>();
  for (const b of props.composite.branches ?? []) childByBranch.set(b.branch, b.child);

  const size = props.size;
  const c = size / 2;
  const rp = c * 0.6; // inner pie radius (parent)
  const rc = c; // outer ring radius (children)

  const arcs: Arc[] = [];
  const legend: LegendRow[] = [];
  let legendY = 0;
  let childColor = 0;

  const pushLegend = (
    indent: boolean,
    header: boolean,
    colorClass: string,
    fill: string,
    text: string,
  ) => {
    legend.push({ y: legendY, indent, header, colorClass, fill, text });
    legendY += ROW;
  };

  let angle = -Math.PI / 2;
  parentSlices.forEach((s) => {
    const frac = s.value / parentTotal;
    const a0 = angle;
    const a1 = angle + frac * Math.PI * 2;
    angle = a1;

    const token = facetColorToken(props.facets, parentSource, s.raw);
    arcs.push({
      d: pieSlice(c, rp, a0, a1),
      colorClass: token ? `expr-text-${token}` : "",
      fill: token ? "" : CHART_PALETTE[arcs.length % CHART_PALETTE.length],
    });
    pushLegend(
      false,
      false,
      token ? `expr-text-${token}` : "",
      token ? "" : CHART_PALETTE[(arcs.length - 1) % CHART_PALETTE.length],
      `${s.label} - ${fmtNum(s.value)} (${fmtPct(s.pct)}%)`,
    );

    const child = childByBranch.get(s.raw);
    if (!child) return; // solid leaf: inner slice only

    const childSlices = slicesOf(child, props.childMeasureIndex);
    const childTotal = childSlices.reduce((a, x) => a + x.value, 0);
    if (childTotal <= 0) return;

    let ca = a0;
    childSlices.forEach((cs) => {
      const share = cs.value / childTotal; // share of the branch, sums to 100%
      const ca1 = ca + share * frac * Math.PI * 2; // fill the parent arc proportionally
      const fill = CHART_PALETTE[childColor % CHART_PALETTE.length];
      arcs.push({ d: ringSeg(c, rp, rc, ca, ca1), colorClass: "", fill });
      // Raw value plus its server-computed share of the branch, so the legend
      // speaks the same language as the arc (the shares roll up to the slice).
      pushLegend(true, false, "", fill, `${cs.label} - ${fmtNum(cs.value)} (${fmtPct(cs.pct)}%)`);
      childColor++;
      ca = ca1;
    });
  });

  // viewBox width sized to the longest legend line.
  const maxChars = Math.max(0, ...legend.map((l) => l.text.length + (l.indent ? 3 : 0)));
  const legendW = SWATCH + 8 + Math.round(maxChars * 6.2);
  const W = Math.max(size, legendW) + PAD * 2;
  const pieX = (W - size) / 2;
  const legendY0 = PAD + size + GAP;
  legend.forEach((l) => (l.y += legendY0));
  const H = legendY0 + legend.length * ROW + PAD;
  return { arcs, legend, pieX, W, H, c };
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
          :key="`arc-${i}`"
          :d="a.d"
          :class="['stat-pie-slice', a.colorClass]"
          :style="{ fill: a.colorClass ? 'currentColor' : a.fill }"
          fill-opacity="0.82"
        />
      </g>
      <g v-for="(l, i) in view.legend" :key="`leg-${i}`">
        <rect
          :x="PAD + (l.indent ? 16 : 0)"
          :y="l.y"
          :width="SWATCH"
          :height="SWATCH"
          rx="2"
          :class="l.colorClass"
          :style="{ fill: l.colorClass ? 'currentColor' : l.fill }"
        />
        <text
          :x="PAD + (l.indent ? 16 : 0) + SWATCH + 6"
          :y="l.y + SWATCH - 2"
          class="stat-bar-label"
        >{{ l.text }}</text>
      </g>
    </svg>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
