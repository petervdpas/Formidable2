<script setup lang="ts" generic="T">
import { computed, ref } from "vue";

// VisualGraph is a generic, reusable DAG renderer. Each row owns
// its own lane segment (CSS-driven), so the lane line stretches
// naturally when a row expands - no SVG-y-coord recomputation.
// Single-lane v1 (column 0); multi-lane fork/merge rendering is
// a follow-up.
//
// Generic over the per-node `data` payload. The default scoped slot
// receives `{ node, expanded, toggle }` so consumers can render row
// content however they like and (when `expandable` is true) include
// their own chevron / handle that calls `toggle()`. The optional
// `details` slot is rendered below the row when `expanded` is true.
//
// Today this powers Collaboration → Commit Graph; the same component
// can later render plugin dependency chains, journal trails, or any
// other parent-id DAG.

export interface GraphNode<TData = unknown> {
  id: string;
  parents: string[];
  data?: TData;
}

const props = withDefaults(
  defineProps<{
    nodes: GraphNode<T>[];
    /** Lane line color - falls back to a neutral border tone. */
    laneColor?: string;
    /** Dot fill - falls back to the theme accent. */
    dotColor?: string;
    /** Dot radius in px. */
    dotRadius?: number;
    /** When true, rows expose `toggle` and the `details` slot is
     *  rendered for expanded rows. Default false. */
    expandable?: boolean;
  }>(),
  {
    laneColor: "var(--color-border-strong, #5b6377)",
    dotColor: "var(--color-accent, #4a90e2)",
    dotRadius: 5,
  },
);

const emit = defineEmits<{
  (e: "node-click", id: string): void;
  (e: "expand", id: string): void;
  (e: "collapse", id: string): void;
}>();

const expandedIds = ref<Set<string>>(new Set());

function isExpanded(id: string): boolean {
  return expandedIds.value.has(id);
}

function toggle(id: string): void {
  const next = new Set(expandedIds.value);
  if (next.has(id)) {
    next.delete(id);
    emit("collapse", id);
  } else {
    next.add(id);
    emit("expand", id);
  }
  expandedIds.value = next;
}

// The lane indicator for each row is a flex column on the left:
// a vertical line (CSS pseudo-element fills 100% of row height) +
// a centered dot (positioned absolutely so expansion doesn't
// reposition it). Each row connects to the next via the line in
// the next row when there's a parent/child relationship.
//
// Lane drop test: a connecting line continues into row[i+1] when
// row[i+1].id is in row[i].parents. We reflect that on row[i+1]
// by adding a CSS class that fills the line above the dot.
const laneFlags = computed(() => {
  return props.nodes.map((node, i) => {
    const above = i > 0 && props.nodes[i - 1].parents.includes(node.id);
    const below = i < props.nodes.length - 1 && node.parents.includes(props.nodes[i + 1].id);
    return { above, below };
  });
});

function onRowClick(id: string) {
  emit("node-click", id);
}
</script>

<template>
  <ul class="visual-graph">
    <li
      v-for="(node, i) in nodes"
      :key="node.id"
      class="visual-graph__row"
      :class="{
        'visual-graph__row--expanded': isExpanded(node.id),
        'visual-graph__row--lane-above': laneFlags[i].above,
        'visual-graph__row--lane-below': laneFlags[i].below,
      }"
    >
      <div
        class="visual-graph__lane"
        :style="{
          '--graph-lane-color': laneColor,
          '--graph-dot-color': dotColor,
          '--graph-dot-radius': dotRadius + 'px',
        }"
      >
        <span class="visual-graph__dot" />
      </div>
      <div class="visual-graph__body">
        <div class="visual-graph__header" @click="onRowClick(node.id)">
          <button
            v-if="expandable"
            type="button"
            class="visual-graph__chevron"
            :aria-expanded="isExpanded(node.id)"
            @click.stop="toggle(node.id)"
          >
            <span :class="['visual-graph__chevron-icon', { 'is-open': isExpanded(node.id) }]">›</span>
          </button>
          <slot
            :node="node"
            :expanded="isExpanded(node.id)"
            :toggle="() => toggle(node.id)"
          >
            <span class="muted small">{{ node.id }}</span>
          </slot>
        </div>
        <div v-if="expandable && isExpanded(node.id)" class="visual-graph__details">
          <slot name="details" :node="node" />
        </div>
      </div>
    </li>
  </ul>
</template>
