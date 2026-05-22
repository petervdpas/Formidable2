<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import MonitorTimeSeries from "./MonitorTimeSeries.vue";
import MonitorBars from "./MonitorBars.vue";
import { Result } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/monitor/models";
import { backendErrMessage } from "../../utils/backendError";
import { runTile, type MonitorTile } from "../../composables/monitorTiles";

const props = defineProps<{
  tile: MonitorTile;
}>();

const data = ref<Result | null>(null);
const loading = ref(false);
const errorMsg = ref("");

async function refresh() {
  loading.value = true;
  errorMsg.value = "";
  try {
    data.value = await runTile(props.tile);
  } catch (err) {
    errorMsg.value = backendErrMessage(err);
    data.value = null;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);
// If the tile config itself changes (future user-edited tiles),
// reload data.
watch(
  () => props.tile.id,
  () => refresh(),
);

defineExpose({ refresh });
</script>

<template>
  <section class="monitor-tile">
    <header class="monitor-tile-header">
      <!-- Title block is the drag handle (mouse down here starts a
           drag; refresh button below is excluded). Uses the global
           .dnd-handle class from dnd.css - no monitor-specific dnd
           visuals. -->
      <div class="monitor-tile-title dnd-handle">
        <h3>{{ tile.title }}</h3>
        <p v-if="tile.description" class="monitor-tile-desc">
          {{ tile.description }}
        </p>
      </div>
      <button
        type="button"
        class="btn btn-icon"
        :disabled="loading"
        :aria-label="`Refresh ${tile.title}`"
        @click="refresh"
      >
        <i class="fa-solid fa-rotate-right" />
      </button>
    </header>

    <div class="monitor-tile-body">
      <p v-if="loading" class="monitor-empty">Loading…</p>
      <p v-else-if="errorMsg" class="monitor-error">{{ errorMsg }}</p>
      <template v-else-if="tile.chart === 'timeseries'">
        <MonitorTimeSeries :result="data" />
      </template>
      <template v-else-if="tile.chart === 'bars'">
        <MonitorBars :result="data" />
      </template>
    </div>
  </section>
</template>
