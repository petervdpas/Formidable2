<script setup lang="ts">
import { computed } from "vue";
import draggable from "vuedraggable";
import MonitorTile from "./MonitorTile.vue";
import type { MonitorTile as MonitorTileConfig } from "../../composables/monitorTiles";

const props = defineProps<{
  tiles: MonitorTileConfig[];
}>();

const emit = defineEmits<{
  (e: "update:tiles", value: MonitorTileConfig[]): void;
}>();

// Writable model for vuedraggable's v-model. Mirrors the FormLoop
// pattern: get returns the current array, set re-emits up the tree
// so the parent owns persistence (localStorage today, user config
// later).
const draggableTiles = computed<MonitorTileConfig[]>({
  get: () => props.tiles,
  set: (next) => emit("update:tiles", next),
});
</script>

<template>
  <draggable
    v-model="draggableTiles"
    tag="div"
    class="monitor-board"
    handle=".dnd-handle"
    :animation="150"
    ghost-class="dnd-ghost"
    chosen-class="dnd-chosen"
    drag-class="dnd-drag"
    :item-key="(t: MonitorTileConfig) => t.id"
  >
    <template #item="{ element: tile }">
      <MonitorTile :tile="tile" />
    </template>
  </draggable>
</template>
