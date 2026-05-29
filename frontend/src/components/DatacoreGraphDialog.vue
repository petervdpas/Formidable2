<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ForceGraph from "./ForceGraph.vue";
import {
  Service as DatacoreSvc,
  type GraphNode,
  type GraphEdge,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/datacore";
import { backendErrMessage } from "../utils/backendError";

// A live node-link view of the datacore tensor rooted at the selected record:
// the record plus its loop rows and links, expanded one hop at a time by
// clicking a node. Read-only; builds a fresh tensor from the template's forms
// and reads off the reference graph from that record outward.
const props = defineProps<{ open: boolean; templateFilename: string; record: string }>();
const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const nodes = ref<GraphNode[]>([]);
const edges = ref<GraphEdge[]>([]);
const expanded = ref<Set<string>>(new Set());
const loading = ref(false);
const errorMsg = ref("");

function edgeKey(e: GraphEdge): string {
  return `${e.source}${e.target}${e.field}`;
}

async function fetchFrom(id: string, depth: number): Promise<boolean> {
  loading.value = true;
  errorMsg.value = "";
  try {
    const g = await DatacoreSvc.GraphFrom(props.templateFilename, id, depth);
    const haveNodes = new Set(nodes.value.map((n) => n.id));
    for (const n of g.nodes) if (!haveNodes.has(n.id)) nodes.value.push(n);
    const haveEdges = new Set(edges.value.map(edgeKey));
    for (const e of g.edges) if (!haveEdges.has(edgeKey(e))) edges.value.push(e);
    return true;
  } catch (err) {
    errorMsg.value = backendErrMessage(err);
    return false;
  } finally {
    loading.value = false;
  }
}

async function reset() {
  nodes.value = [];
  edges.value = [];
  expanded.value = new Set();
  if (!props.record) return;
  if (await fetchFrom(props.record, 1)) expanded.value.add(props.record);
}

async function onNodeClick(id: string) {
  // Field nodes are value leaves, not identities: nothing to unfold.
  const node = nodes.value.find((n) => n.id === id);
  if (!node || node.kind === "field") return;
  if (expanded.value.has(id)) return;
  expanded.value.add(id);
  await fetchFrom(id, 1);
}

watch(
  () => props.open,
  (open) => {
    if (open) reset();
    else {
      nodes.value = [];
      edges.value = [];
    }
  },
);
</script>

<template>
  <Modal :open="open" :title="t('datacore.graph_title')" width="860px" maximizable @close="emit('close')">
    <div class="datacore-graph">
      <div class="datacore-graph__bar">
        <span class="datacore-graph__hint form-description">{{ t('datacore.click_hint') }}</span>
        <button type="button" class="tool-btn" :disabled="loading || !record" @click="reset">
          {{ t('datacore.reset') }}
        </button>
        <span v-if="nodes.length" class="datacore-graph__count">
          {{ t('datacore.count', { nodes: nodes.length, edges: edges.length }) }}
        </span>
        <span class="datacore-graph__legend">
          <i class="datacore-dot datacore-dot--root"></i>{{ t('datacore.legend_root') }}
          <i class="datacore-dot datacore-dot--row"></i>{{ t('datacore.legend_row') }}
          <i class="datacore-dot datacore-dot--field"></i>{{ t('datacore.legend_field') }}
        </span>
      </div>

      <p v-if="!record" class="form-description">{{ t('datacore.no_record') }}</p>
      <p v-else-if="errorMsg" class="datacore-graph__error">{{ errorMsg }}</p>
      <p v-else-if="!nodes.length && loading" class="form-description">{{ t('datacore.loading') }}</p>
      <p v-else-if="!nodes.length" class="form-description">{{ t('datacore.empty') }}</p>
      <ForceGraph v-else :nodes="nodes" :edges="edges" @node-click="onNodeClick" />
    </div>
  </Modal>
</template>
