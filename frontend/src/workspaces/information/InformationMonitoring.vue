<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import MonitorBoard from "../../components/monitor/MonitorBoard.vue";
import {
  defaultMonitorTiles,
  loadTileOrder,
  saveTileOrder,
  type MonitorTile,
} from "../../composables/monitorTiles";

const { t } = useI18n();

// v1: hardcoded default list, then apply the user's saved order on
// top. When the user drags, persist the new order. Future: backend-
// supplied tile set so users can add/remove their own.
const tiles = ref<MonitorTile[]>(loadTileOrder(defaultMonitorTiles()));

watch(
  tiles,
  (next) => saveTileOrder(next),
  { deep: false },
);
</script>

<template>
  <p class="section-info">{{ t('workspace.information.monitoring.info') }}</p>
  <MonitorBoard v-model:tiles="tiles" />
</template>
