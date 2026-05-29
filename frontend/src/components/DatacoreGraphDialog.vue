<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ForceGraph from "./ForceGraph.vue";
import { Service as DatacoreSvc, type Graph } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/datacore";
import { backendErrMessage } from "../utils/backendError";

// A live node-link view of the datacore tensor for one template: records and
// their loop rows / links as nodes, refs as edges. Read-only; it builds a
// fresh tensor from the template's forms and reads off its reference graph.
const props = defineProps<{ open: boolean; templateFilename: string }>();
const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const graph = ref<Graph | null>(null);
const loading = ref(false);
const errorMsg = ref("");
const cap = ref(200);

async function load() {
  if (!props.templateFilename) return;
  loading.value = true;
  errorMsg.value = "";
  try {
    graph.value = await DatacoreSvc.Graph(props.templateFilename, cap.value);
  } catch (err) {
    errorMsg.value = backendErrMessage(err);
    graph.value = null;
  } finally {
    loading.value = false;
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) load();
    else graph.value = null;
  },
);
</script>

<template>
  <Modal :open="open" :title="t('datacore.graph_title')" width="860px" maximizable @close="emit('close')">
    <div class="datacore-graph">
      <div class="datacore-graph__bar">
        <label class="datacore-graph__cap">
          {{ t('datacore.node_cap') }}
          <input v-model.number="cap" type="number" min="10" max="2000" step="10" />
        </label>
        <button type="button" class="tool-btn" :disabled="loading || !templateFilename" @click="load">
          {{ t('datacore.reload') }}
        </button>
        <span v-if="graph" class="datacore-graph__count">
          {{ t('datacore.count', { nodes: graph.nodes.length, edges: graph.edges.length }) }}
        </span>
        <span class="datacore-graph__legend">
          <i class="datacore-dot datacore-dot--root"></i>{{ t('datacore.legend_root') }}
          <i class="datacore-dot datacore-dot--row"></i>{{ t('datacore.legend_row') }}
        </span>
      </div>

      <p v-if="graph?.capped" class="datacore-graph__capped form-description">
        {{ t('datacore.capped', { n: cap }) }}
      </p>

      <p v-if="loading" class="form-description">{{ t('datacore.loading') }}</p>
      <p v-else-if="errorMsg" class="datacore-graph__error">{{ errorMsg }}</p>
      <p v-else-if="!graph || graph.nodes.length === 0" class="form-description">{{ t('datacore.empty') }}</p>
      <ForceGraph v-else :nodes="graph.nodes" :edges="graph.edges" />
    </div>
  </Modal>
</template>
