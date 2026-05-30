<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ForceGraph from "./ForceGraph.vue";
import {
  Service as DatacoreSvc,
  type GraphNode,
  type GraphEdge,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/datacore";
import { backendErrMessage } from "../utils/backendError";

// A live node-link view of the datacore tensor rooted at the selected record.
// A Detail level controls how much of the flower shows: 1 = the record only,
// 2 = its fields (incl. table/list/link field nodes), 3 = the rows under them.
// Clicking a row or a linked record unfolds that node further. Read-only.
const props = defineProps<{ open: boolean; templateFilename: string; record: string }>();
const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const nodes = ref<GraphNode[]>([]);
const edges = ref<GraphEdge[]>([]);
const expanded = ref<Set<string>>(new Set());
const loading = ref(false);
const errorMsg = ref("");
const level = ref(2); // 1 = record, 2 = fields, 3 = rows

const LEVEL_KEYS = { 1: "datacore.level_root", 2: "datacore.level_fields", 3: "datacore.level_rows" } as const;
const levelName = computed(() => t(LEVEL_KEYS[level.value as 1 | 2 | 3]));

function edgeKey(e: GraphEdge): string {
  return `${e.source}${e.target}${e.field}`;
}

async function fetchInto(id: string, detail: number): Promise<boolean> {
  loading.value = true;
  errorMsg.value = "";
  try {
    const g = await DatacoreSvc.GraphFrom(props.templateFilename, id, detail);
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

async function loadRoot() {
  nodes.value = [];
  edges.value = [];
  expanded.value = new Set();
  if (!props.record) return;
  // Backend level is 0-based: record=0, fields=1, rows=2.
  if (await fetchInto(props.record, level.value - 1)) expanded.value.add(props.record);
}

function setLevel(l: number) {
  level.value = Math.min(3, Math.max(1, l));
  loadRoot();
}

async function onNodeClick(id: string) {
  // Field nodes are value leaves; only rows and linked records unfold further.
  const node = nodes.value.find((n) => n.id === id);
  if (!node || node.kind === "field") return;
  if (expanded.value.has(id)) return;
  expanded.value.add(id);
  await fetchInto(id, 2); // reveal the clicked node's fields and rows
}

watch(
  () => props.open,
  (open) => {
    if (open) loadRoot();
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
        <span class="datacore-graph__detail">
          {{ t('datacore.detail') }}
          <button type="button" class="tool-btn" :disabled="loading || level <= 1" @click="setLevel(level - 1)">−</button>
          <span class="datacore-graph__level">{{ levelName }}</span>
          <button type="button" class="tool-btn" :disabled="loading || level >= 3" @click="setLevel(level + 1)">+</button>
        </span>
        <button type="button" class="tool-btn" :disabled="loading || !record" @click="loadRoot">
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
