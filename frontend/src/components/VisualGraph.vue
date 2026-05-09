<script setup lang="ts" generic="T">
import { computed } from "vue";

// VisualGraph is a generic, reusable DAG renderer. It draws a single
// vertical lane on the left (dot per node + connecting line where
// adjacent rows are parent/child) and lets the consumer fill the
// per-row content via a scoped slot. No git-specific knowledge —
// today it visualises commit history, but the same component can
// render plugin dependency chains, journal trails, anything else
// shaped as nodes-with-parent-ids.
//
// Lane layout (v1): single lane, column 0. We connect successive
// rows when row[i+1] is a parent of row[i] — the common linear-
// history case renders cleanly. For branchy histories the row order
// still reflects topological newest-first; the lane line just
// becomes a series of disjoint segments (still readable). Multi-
// lane layout for visible forks/merges is a follow-up.

export interface GraphNode<TData = unknown> {
  id: string;
  parents: string[];
  data?: TData;
}

const props = withDefaults(
  defineProps<{
    nodes: GraphNode<T>[];
    /** Row height in px. Affects both DOM and SVG line geometry. */
    rowHeight?: number;
    /** Lane line color. Defaults to a neutral gray; consumers can override. */
    laneColor?: string;
    /** Dot fill — same default note. */
    dotColor?: string;
    /** Dot radius. */
    dotRadius?: number;
  }>(),
  {
    rowHeight: 36,
    laneColor: "var(--color-border-strong, #5b6377)",
    dotColor: "var(--color-accent, #4a90e2)",
    dotRadius: 5,
  },
);

const emit = defineEmits<{
  (e: "node-click", id: string): void;
}>();

// laneX and dotCx are kept simple — single-lane v1 always sits at
// column 0. The component reserves a fixed gutter on the left of
// each row for the SVG so the slot content lines up consistently.
const laneX = 16;

// segments returns the y-pairs (top → bottom) for the connecting
// lane lines. A line is drawn between row i and row i+1 when row
// i+1's id appears in row i's parents — so the line represents
// "this commit's parent is the one below it".
const segments = computed(() => {
  const out: Array<{ y1: number; y2: number }> = [];
  const half = props.rowHeight / 2;
  for (let i = 0; i < props.nodes.length - 1; i++) {
    const cur = props.nodes[i];
    const next = props.nodes[i + 1];
    if (cur.parents.includes(next.id)) {
      out.push({
        y1: i * props.rowHeight + half,
        y2: (i + 1) * props.rowHeight + half,
      });
    }
  }
  return out;
});

const totalHeight = computed(() => props.nodes.length * props.rowHeight);

function onRowClick(id: string) {
  emit("node-click", id);
}
</script>

<template>
  <div class="visual-graph" :style="{ '--graph-row-height': rowHeight + 'px' }">
    <svg
      class="visual-graph__lane"
      :width="laneX * 2"
      :height="totalHeight"
      aria-hidden="true"
    >
      <line
        v-for="(seg, i) in segments"
        :key="i"
        :x1="laneX"
        :y1="seg.y1"
        :x2="laneX"
        :y2="seg.y2"
        :stroke="laneColor"
        stroke-width="2"
      />
      <circle
        v-for="(node, i) in nodes"
        :key="node.id"
        :cx="laneX"
        :cy="i * rowHeight + rowHeight / 2"
        :r="dotRadius"
        :fill="dotColor"
      />
    </svg>
    <ul class="visual-graph__rows">
      <li
        v-for="node in nodes"
        :key="node.id"
        class="visual-graph__row"
        @click="onRowClick(node.id)"
      >
        <slot :node="node">
          <span class="muted small">{{ node.id }}</span>
        </slot>
      </li>
    </ul>
  </div>
</template>
