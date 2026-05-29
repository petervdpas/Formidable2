<script setup lang="ts">
import { shallowRef, triggerRef, onBeforeUnmount, useTemplateRef, watch } from "vue";

// A small force-directed node-link renderer: nodes repel, edges pull like
// springs, a gentle gravity keeps the web centered, and the simulation cools
// to rest. Drag a node to pin and re-heat it. Hand-rolled SVG, consistent
// with the other charts; no graph library.

interface InNode {
  id: string;
  label: string;
  kind: string; // "root" | "row"
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

let raf = 0;
let alpha = 0;
let dragging = -1;

const REPULSION = 2600;
const SPRING = 0.02;
const REST = 90;
const GRAVITY = 0.015;
const FRICTION = 0.85;
const MIN_ALPHA = 0.02;

function build() {
  const cx = props.width / 2;
  const cy = props.height / 2;
  const n = props.nodes.length;
  const r = Math.min(cx, cy) * 0.8;
  // Golden-angle seed layout so nodes start spread out, not stacked.
  const golden = Math.PI * (3 - Math.sqrt(5));
  sim.value = props.nodes.map((node, i) => {
    const t = n > 1 ? i / (n - 1) : 0;
    const rad = r * Math.sqrt(t);
    const ang = i * golden;
    return { ...node, x: cx + rad * Math.cos(ang), y: cy + rad * Math.sin(ang), vx: 0, vy: 0 };
  });
  const idx = new Map<string, number>();
  props.nodes.forEach((node, i) => idx.set(node.id, i));
  links.value = props.edges
    .map((e) => ({ a: idx.get(e.source) ?? -1, b: idx.get(e.target) ?? -1, field: e.field }))
    .filter((l) => l.a >= 0 && l.b >= 0);
  reheat(1);
}

function reheat(to: number) {
  alpha = Math.max(alpha, to);
  if (!raf) raf = requestAnimationFrame(step);
}

function step() {
  const nodes = sim.value;
  const cx = props.width / 2;
  const cy = props.height / 2;

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
      const f = REPULSION / d2;
      const d = Math.sqrt(d2);
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
    a.x = Math.max(12, Math.min(props.width - 12, a.x));
    a.y = Math.max(12, Math.min(props.height - 12, a.y));
  }

  triggerRef(sim);

  if (dragging < 0) alpha *= 0.97;
  if (alpha > MIN_ALPHA || dragging >= 0) {
    raf = requestAnimationFrame(step);
  } else {
    raf = 0;
  }
}

function toLocal(e: PointerEvent): { x: number; y: number } {
  const svg = svgRef.value;
  if (!svg) return { x: 0, y: 0 };
  const rect = svg.getBoundingClientRect();
  return {
    x: ((e.clientX - rect.left) / rect.width) * props.width,
    y: ((e.clientY - rect.top) / rect.height) * props.height,
  };
}

function onDown(i: number, e: PointerEvent) {
  dragging = i;
  (e.target as Element).setPointerCapture?.(e.pointerId);
  reheat(0.4);
}
function onMove(e: PointerEvent) {
  if (dragging < 0) return;
  const p = toLocal(e);
  const a = sim.value[dragging];
  a.x = p.x;
  a.y = p.y;
  a.vx = 0;
  a.vy = 0;
  triggerRef(sim);
}
function onUp() {
  if (dragging < 0) return;
  dragging = -1;
  reheat(0.2);
}

function short(label: string): string {
  const tail = label.includes("#") ? label.slice(label.lastIndexOf("#") + 1) : label;
  return tail.length > 18 ? tail.slice(0, 17) + "…" : tail;
}

watch(() => [props.nodes, props.edges], build, { immediate: true });

onBeforeUnmount(() => {
  if (raf) cancelAnimationFrame(raf);
});
</script>

<template>
  <svg
    ref="svg"
    class="force-graph"
    :viewBox="`0 0 ${width} ${height}`"
    preserveAspectRatio="xMidYMid meet"
    @pointermove="onMove"
    @pointerup="onUp"
    @pointerleave="onUp"
  >
    <line
      v-for="(l, i) in links"
      :key="`e${i}`"
      class="force-edge"
      :x1="sim[l.a].x"
      :y1="sim[l.a].y"
      :x2="sim[l.b].x"
      :y2="sim[l.b].y"
    >
      <title>{{ l.field }}</title>
    </line>
    <g
      v-for="(node, i) in sim"
      :key="node.id"
      :class="['force-node', `force-node--${node.kind}`]"
      :transform="`translate(${node.x}, ${node.y})`"
      @pointerdown="onDown(i, $event)"
    >
      <circle :r="node.kind === 'root' ? 9 : 5" />
      <text :x="node.kind === 'root' ? 12 : 8" y="4">{{ short(node.label) }}</text>
      <title>{{ node.label }}</title>
    </g>
  </svg>
</template>
