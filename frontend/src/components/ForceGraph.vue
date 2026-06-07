<script setup lang="ts">
import { shallowRef, ref, computed, triggerRef, onBeforeUnmount, onMounted, useTemplateRef, watch } from "vue";

// A small force-directed node-link renderer: nodes repel, edges pull like
// springs, gravity keeps the web centered, and the simulation cools to rest.
// Drag a node to pin it; a click (no drag) emits node-click so the caller can
// unfold it. Drag empty space to pan, wheel to zoom (to the cursor). Hovering a
// node reveals its full detail in a tooltip, so persistent labels stay short.
// When the node/edge props grow, existing positions are preserved and only new
// nodes are seeded, so expanding doesn't reshuffle the layout. Hand-rolled SVG.
//
// The canvas measures its container (ResizeObserver) and lays out in that real
// pixel box, so it grows with the dialog instead of scaling a fixed viewBox.

interface NodeTable {
  title: string;
  header?: string[]; // column labels, when known
  rows: string[][]; // cell columns per row
  more: number; // rows beyond the shown cap
}
interface InNode {
  id: string;
  label: string;
  kind: string; // "root" | "focus" | "related-cross" | "row" | "field"
  detail?: string; // full info shown on hover (defaults to label)
  table?: NodeTable; // a table container renders its rows as a grid on hover
}
interface InEdge {
  source: string;
  target: string;
  field: string;
}

const props = withDefaults(
  defineProps<{
    nodes: InNode[];
    edges: InEdge[];
    width?: number;
    height?: number;
  }>(),
  { width: 760, height: 520 },
);

const emit = defineEmits<{ (e: "node-click", id: string): void }>();

interface SimNode extends InNode {
  x: number;
  y: number;
  vx: number;
  vy: number;
}
interface SimEdge {
  a: number;
  b: number;
  field: string;
}

const sim = shallowRef<SimNode[]>([]);
const links = shallowRef<SimEdge[]>([]);
const svgRef = useTemplateRef<SVGSVGElement>("svg");
const wrapRef = useTemplateRef<HTMLDivElement>("wrap");

// Live canvas size, measured from the container; seeded from the props.
const w = ref(props.width);
const h = ref(props.height);
let ro: ResizeObserver | null = null;

const zoom = ref(1);
const panX = ref(0);
const panY = ref(0);

// Hover tooltip: the node under the pointer and the cursor position (relative to
// the wrap), so the detail panel follows the pointer.
const hoverId = ref<string | null>(null);
const tipX = ref(0);
const tipY = ref(0);
const hoverDetail = computed(() => {
  if (!hoverId.value) return "";
  const n = sim.value.find((s) => s.id === hoverId.value);
  return n ? n.detail || n.label : "";
});
const hoverHead = computed(() => hoverDetail.value.split("\n")[0] ?? "");
const hoverBody = computed(() => hoverDetail.value.split("\n").slice(1).join("\n"));
const hoverTable = computed<NodeTable | null>(() => {
  if (!hoverId.value) return null;
  const n = sim.value.find((s) => s.id === hoverId.value);
  return n?.table ?? null;
});

// Position the tooltip near the pointer but keep it inside the canvas: flip to
// the left/top half when the cursor is past the midpoint, and cap its size to
// the available space so it grows with the dialog instead of overflowing.
const tipStyle = computed<Record<string, string>>(() => {
  const s: Record<string, string> = {};
  if (tipX.value > w.value / 2) s.right = `${Math.max(0, w.value - tipX.value + 14)}px`;
  else s.left = `${tipX.value + 14}px`;
  if (tipY.value > h.value / 2) s.bottom = `${Math.max(0, h.value - tipY.value + 14)}px`;
  else s.top = `${tipY.value + 14}px`;
  s.maxWidth = `${Math.max(200, w.value - 28)}px`;
  s.maxHeight = `${Math.max(120, h.value - 28)}px`;
  return s;
});

let raf = 0;
let alpha = 0;
let autofit = true;
let dragging = -1;
let downX = 0;
let downY = 0;
let moved = false;
let panning = false;
let panStartX = 0;
let panStartY = 0;
let panOrigX = 0;
let panOrigY = 0;

const REPULSION = 2600;
const SPRING = 0.02;
const REST = 90;
const GRAVITY = 0.015;
const FRICTION = 0.85;
const MIN_ALPHA = 0.02;
const GOLDEN = Math.PI * (3 - Math.sqrt(5));

function merge() {
  const cx = w.value / 2;
  const cy = h.value / 2;
  const prev = new Map(sim.value.map((n) => [n.id, n]));
  sim.value = props.nodes.map((node, i) => {
    const ex = prev.get(node.id);
    if (ex) return { ...node, x: ex.x, y: ex.y, vx: ex.vx, vy: ex.vy };
    const rad = 30 + (i % 9) * 7;
    const ang = i * GOLDEN;
    return { ...node, x: cx + rad * Math.cos(ang), y: cy + rad * Math.sin(ang), vx: 0, vy: 0 };
  });
  const idx = new Map<string, number>();
  props.nodes.forEach((node, i) => idx.set(node.id, i));
  links.value = props.edges
    .map((e) => ({ a: idx.get(e.source) ?? -1, b: idx.get(e.target) ?? -1, field: e.field }))
    .filter((l) => l.a >= 0 && l.b >= 0);
  autofit = true;
  reheat(0.8);
}

function reheat(to: number) {
  alpha = Math.max(alpha, to);
  if (!raf) raf = requestAnimationFrame(step);
}

function step() {
  const nodes = sim.value;
  const cx = w.value / 2;
  const cy = h.value / 2;

  for (let i = 0; i < nodes.length; i++) {
    const a = nodes[i];
    let fx = (cx - a.x) * GRAVITY;
    let fy = (cy - a.y) * GRAVITY;
    for (let j = 0; j < nodes.length; j++) {
      if (i === j) continue;
      const b = nodes[j];
      let dx = a.x - b.x;
      let dy = a.y - b.y;
      let d2 = dx * dx + dy * dy;
      if (d2 < 0.01) {
        dx = (i - j) * 0.1;
        dy = 0.1;
        d2 = dx * dx + dy * dy;
      }
      const d = Math.sqrt(d2);
      const f = REPULSION / d2;
      fx += (dx / d) * f;
      fy += (dy / d) * f;
    }
    a.vx = (a.vx + fx) * FRICTION;
    a.vy = (a.vy + fy) * FRICTION;
  }

  for (const l of links.value) {
    const a = nodes[l.a];
    const b = nodes[l.b];
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const d = Math.sqrt(dx * dx + dy * dy) || 1;
    const f = (d - REST) * SPRING;
    const ux = (dx / d) * f;
    const uy = (dy / d) * f;
    a.vx += ux;
    a.vy += uy;
    b.vx -= ux;
    b.vy -= uy;
  }

  for (let i = 0; i < nodes.length; i++) {
    if (i === dragging) continue;
    const a = nodes[i];
    a.x += a.vx * alpha;
    a.y += a.vy * alpha;
    a.x = Math.max(12, Math.min(w.value - 12, a.x));
    a.y = Math.max(12, Math.min(h.value - 12, a.y));
  }

  triggerRef(sim);

  if (dragging < 0) alpha *= 0.97;
  if (alpha > MIN_ALPHA || dragging >= 0) {
    raf = requestAnimationFrame(step);
  } else {
    raf = 0;
    if (autofit) {
      autofit = false;
      fitView();
    }
  }
}

// Scale and center the view so the node bounding box fills the canvas with a
// margin. Runs once after the layout settles (autofit), and on the fit button.
function fitView() {
  const nodes = sim.value;
  if (!nodes.length) return;
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const n of nodes) {
    // Include the label's extent to the right so text isn't clipped at fit.
    const labelW = 12 + short(n.label).length * 6.2;
    if (n.x - 10 < minX) minX = n.x - 10;
    if (n.y - 10 < minY) minY = n.y - 10;
    if (n.x + labelW > maxX) maxX = n.x + labelW;
    if (n.y + 10 > maxY) maxY = n.y + 10;
  }
  const pad = 30;
  const bw = Math.max(1, maxX - minX);
  const bh = Math.max(1, maxY - minY);
  const z = Math.min((w.value - 2 * pad) / bw, (h.value - 2 * pad) / bh, 2.4);
  zoom.value = z;
  panX.value = w.value / 2 - ((minX + maxX) / 2) * z;
  panY.value = h.value / 2 - ((minY + maxY) / 2) * z;
}

// Map a pointer event to graph-space coordinates, inverting pan + zoom.
function toGraph(e: PointerEvent): { x: number; y: number } {
  const svg = svgRef.value;
  if (!svg) return { x: 0, y: 0 };
  const rect = svg.getBoundingClientRect();
  const vbx = ((e.clientX - rect.left) / rect.width) * w.value;
  const vby = ((e.clientY - rect.top) / rect.height) * h.value;
  return { x: (vbx - panX.value) / zoom.value, y: (vby - panY.value) / zoom.value };
}

function setTip(e: PointerEvent) {
  const wrap = wrapRef.value;
  if (!wrap) return;
  const rect = wrap.getBoundingClientRect();
  tipX.value = e.clientX - rect.left;
  tipY.value = e.clientY - rect.top;
}

function onNodeEnter(id: string, e: PointerEvent) {
  if (dragging >= 0 || panning) return;
  hoverId.value = id;
  setTip(e);
}
function onNodeLeave() {
  hoverId.value = null;
}

function onNodeDown(i: number, e: PointerEvent) {
  dragging = i;
  moved = false;
  autofit = false;
  hoverId.value = null; // no tooltip while dragging
  downX = e.clientX;
  downY = e.clientY;
  reheat(0.4);
}
function onSvgDown(e: PointerEvent) {
  if (dragging >= 0) return;
  autofit = false;
  panning = true;
  hoverId.value = null;
  panStartX = e.clientX;
  panStartY = e.clientY;
  panOrigX = panX.value;
  panOrigY = panY.value;
}
function onMove(e: PointerEvent) {
  if (dragging >= 0) {
    if (Math.abs(e.clientX - downX) > 4 || Math.abs(e.clientY - downY) > 4) moved = true;
    const p = toGraph(e);
    const a = sim.value[dragging];
    a.x = p.x;
    a.y = p.y;
    a.vx = 0;
    a.vy = 0;
    triggerRef(sim);
    return;
  }
  if (panning) {
    const svg = svgRef.value;
    const scale = svg ? w.value / svg.getBoundingClientRect().width : 1;
    panX.value = panOrigX + (e.clientX - panStartX) * scale;
    panY.value = panOrigY + (e.clientY - panStartY) * scale;
    return;
  }
  if (hoverId.value) setTip(e); // tooltip follows the pointer
}
function onUp() {
  if (dragging >= 0) {
    const node = sim.value[dragging];
    dragging = -1;
    if (!moved && node) emit("node-click", node.id);
    reheat(0.2);
    return;
  }
  panning = false;
}

function zoomAt(vbx: number, vby: number, factor: number) {
  const gx = (vbx - panX.value) / zoom.value;
  const gy = (vby - panY.value) / zoom.value;
  const z = Math.max(0.2, Math.min(5, zoom.value * factor));
  panX.value = vbx - gx * z;
  panY.value = vby - gy * z;
  zoom.value = z;
}
function onWheel(e: WheelEvent) {
  e.preventDefault();
  autofit = false;
  const svg = svgRef.value;
  if (!svg) return;
  const rect = svg.getBoundingClientRect();
  const vbx = ((e.clientX - rect.left) / rect.width) * w.value;
  const vby = ((e.clientY - rect.top) / rect.height) * h.value;
  zoomAt(vbx, vby, e.deltaY < 0 ? 1.12 : 1 / 1.12);
}
function zoomBy(factor: number) {
  autofit = false;
  zoomAt(w.value / 2, h.value / 2, factor);
}

// Short persistent label; the full text and detail come from the hover tooltip.
function short(label: string): string {
  return label.length > 16 ? label.slice(0, 15) + "…" : label;
}

watch(() => [props.nodes, props.edges], merge, { immediate: true });

onMounted(() => {
  const el = wrapRef.value;
  if (!el || typeof ResizeObserver === "undefined") return;
  ro = new ResizeObserver(() => {
    const rect = el.getBoundingClientRect();
    if (rect.width < 1 || rect.height < 1) return;
    const nw = Math.round(rect.width);
    const nh = Math.round(rect.height);
    if (nw === w.value && nh === h.value) return;
    w.value = nw;
    h.value = nh;
    fitView(); // re-center the existing layout into the new box
    reheat(0.1); // let gravity settle toward the new center
  });
  ro.observe(el);
});

onBeforeUnmount(() => {
  if (raf) cancelAnimationFrame(raf);
  ro?.disconnect();
});
</script>

<template>
  <div ref="wrap" class="force-graph-wrap">
    <svg
      ref="svg"
      class="force-graph"
      :viewBox="`0 0 ${w} ${h}`"
      preserveAspectRatio="xMidYMid meet"
      @pointerdown="onSvgDown"
      @pointermove="onMove"
      @pointerup="onUp"
      @pointerleave="onUp"
      @wheel="onWheel"
    >
      <g :transform="`translate(${panX} ${panY}) scale(${zoom})`">
        <line
          v-for="(l, i) in links"
          :key="`e${i}`"
          class="force-edge"
          :x1="sim[l.a].x"
          :y1="sim[l.a].y"
          :x2="sim[l.b].x"
          :y2="sim[l.b].y"
        />
        <g
          v-for="(node, i) in sim"
          :key="node.id"
          :class="['force-node', `force-node--${node.kind}`]"
          :transform="`translate(${node.x}, ${node.y})`"
          @pointerdown.stop="onNodeDown(i, $event)"
          @pointerenter="onNodeEnter(node.id, $event)"
          @pointerleave="onNodeLeave"
        >
          <circle :r="node.kind === 'row' ? 5 : node.kind === 'field' ? 4 : 9" />
          <text x="11" y="4">{{ short(node.label) }}</text>
        </g>
      </g>
    </svg>
    <div
      v-if="hoverId && (hoverTable || hoverDetail)"
      class="force-tip"
      :style="tipStyle"
    >
      <template v-if="hoverTable">
        <div class="force-tip-head">{{ hoverTable.title }}</div>
        <table class="force-tip-table">
          <thead v-if="hoverTable.header">
            <tr>
              <th v-for="(hd, ci) in hoverTable.header" :key="ci">{{ hd }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(r, ri) in hoverTable.rows" :key="ri">
              <td v-for="(c, ci) in r" :key="ci">{{ c }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="hoverTable.more" class="force-tip-more">+{{ hoverTable.more }} more</div>
      </template>
      <template v-else>
        <div class="force-tip-head">{{ hoverHead }}</div>
        <div v-if="hoverBody" class="force-tip-body">{{ hoverBody }}</div>
      </template>
    </div>
    <div class="force-zoom">
      <button type="button" title="Zoom in" @click="zoomBy(1.2)">+</button>
      <button type="button" title="Zoom out" @click="zoomBy(1 / 1.2)">−</button>
      <button type="button" title="Fit to view" @click="fitView">⤢</button>
    </div>
  </div>
</template>
