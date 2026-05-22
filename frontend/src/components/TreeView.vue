<script setup lang="ts">
/*
 * TreeView - recursive tree navigator. Items can be leaves or
 * branches; branches expand/collapse, leaves emit `select`. Branches
 * that contain the currently-selected leaf auto-expand on mount.
 *
 * Usage:
 *   <TreeView
 *     :items="[
 *       { id: 'a', label: 'A' },
 *       { id: 'help', label: 'Help', children: [
 *           { id: 'help.render', label: 'Render Helpers' },
 *       ]},
 *     ]"
 *     v-model:selectedId="active"
 *   />
 *
 * Selecting a branch toggles its expansion; selecting a leaf updates
 * the v-model. Visual nesting via `--tree-depth` so callers can style
 * indent via CSS.
 *
 * Pairs with `.tree-view` / `.tree-node` / `.tree-row` / `.tree-chevron`
 * in styles/tree-view.css.
 */

import { computed, ref, watch } from "vue";

export type TreeNode = {
  id: string;
  label: string;
  children?: TreeNode[];
  disabled?: boolean;
};

const props = withDefaults(
  defineProps<{
    items: TreeNode[];
    selectedId?: string;
    /** Recursion depth (internal). Top-level callers leave this at 0. */
    depth?: number;
  }>(),
  { selectedId: "", depth: 0 },
);

const emit = defineEmits<{
  (e: "update:selectedId", id: string): void;
  (e: "select", id: string): void;
}>();

// Branch expansion state - local Set keyed by node id. Branches that
// contain the selected leaf auto-expand so a fresh load lands on a
// visible row.
const expanded = ref<Set<string>>(new Set());

function isBranch(node: TreeNode): boolean {
  return Array.isArray(node.children) && node.children.length > 0;
}

function containsSelected(node: TreeNode, selected: string): boolean {
  if (!isBranch(node)) return false;
  for (const child of node.children!) {
    if (child.id === selected) return true;
    if (containsSelected(child, selected)) return true;
  }
  return false;
}

// On selection change (or initial mount via the immediate watcher),
// auto-open every ancestor branch of the selected leaf.
watch(
  () => props.selectedId,
  (sel) => {
    if (!sel) return;
    for (const node of props.items) {
      if (isBranch(node) && containsSelected(node, sel)) {
        expanded.value.add(node.id);
      }
    }
  },
  { immediate: true },
);

function onRowClick(node: TreeNode) {
  if (node.disabled) return;
  if (isBranch(node)) {
    if (expanded.value.has(node.id)) {
      expanded.value.delete(node.id);
    } else {
      expanded.value.add(node.id);
    }
    return;
  }
  emit("update:selectedId", node.id);
  emit("select", node.id);
}

function onChildSelect(id: string) {
  emit("update:selectedId", id);
  emit("select", id);
}

const rootClass = computed(() => (props.depth === 0 ? "tree-view" : "tree-subtree"));
</script>

<template>
  <ul :class="rootClass" :role="depth === 0 ? 'tree' : 'group'">
    <li v-for="node in items" :key="node.id" class="tree-node" role="treeitem">
      <div
        :class="[
          'tree-row',
          {
            active: !isBranch(node) && selectedId === node.id,
            branch: isBranch(node),
            disabled: node.disabled,
            expanded: isBranch(node) && expanded.has(node.id),
          },
        ]"
        :style="{ '--tree-depth': depth }"
        :aria-expanded="isBranch(node) ? expanded.has(node.id) : undefined"
        :aria-selected="!isBranch(node) && selectedId === node.id"
        @click="onRowClick(node)"
      >
        <span v-if="isBranch(node)" class="tree-chevron" aria-hidden="true">
          {{ expanded.has(node.id) ? '▾' : '▸' }}
        </span>
        <span v-else class="tree-chevron tree-chevron--leaf" aria-hidden="true"></span>
        <span class="tree-label">{{ node.label }}</span>
      </div>
      <TreeView
        v-if="isBranch(node) && expanded.has(node.id)"
        :items="node.children!"
        :selected-id="selectedId"
        :depth="depth + 1"
        @update:selected-id="onChildSelect"
      />
    </li>
  </ul>
</template>
