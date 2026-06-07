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

// Relations-only collapses the graph to record-to-record relation edges (the
// intermediate "rel:<to>" field node is contracted away) and drops field/row
// nodes. In that mode clicking a record isolates its connected paths instead of
// unfolding. isolatedId holds the focused record, or null for the whole view.
const relationsOnly = ref(false);
const isolatedId = ref<string | null>(null);
// The record the graph is rooted at (the one you opened); shown in a distinct
// colour so it stands out from the records it relates to.
const rootNodeId = ref<string | null>(null);
const REL_PREFIX = "rel:";

// contractRelations rewrites record -(rel:X)-> [field node] -> record into a
// direct record -> record edge labelled X, keeping only record nodes.
function contractRelations(ns: GraphNode[], es: GraphEdge[]): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const byId = new Map(ns.map((n) => [n.id, n]));
  // A "rel:" edge points record -> relation-field-node; capture that owner+label.
  const relField = new Map<string, { from: string; label: string }>();
  for (const e of es) {
    if (e.field.startsWith(REL_PREFIX)) {
      relField.set(e.target, { from: e.source, label: e.field });
    }
  }
  const outEdges: GraphEdge[] = [];
  const keep = new Set<string>();
  for (const e of es) {
    const rf = relField.get(e.source); // e.source is a relation-field node
    if (!rf) continue;
    const target = byId.get(e.target);
    if (!target || target.kind !== "root") continue;
    outEdges.push({ source: rf.from, target: e.target, field: rf.label });
    keep.add(rf.from);
    keep.add(e.target);
  }
  return { nodes: ns.filter((n) => keep.has(n.id) && n.kind === "root"), edges: outEdges };
}

// isolateAround keeps only the connected subgraph (undirected BFS) containing id.
function isolateAround(ns: GraphNode[], es: GraphEdge[], id: string): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const adj = new Map<string, string[]>();
  const link = (a: string, b: string) => {
    const arr = adj.get(a);
    if (arr) arr.push(b);
    else adj.set(a, [b]);
  };
  for (const e of es) {
    link(e.source, e.target);
    link(e.target, e.source);
  }
  const seen = new Set<string>([id]);
  const queue = [id];
  while (queue.length) {
    const cur = queue.shift() as string;
    for (const nb of adj.get(cur) ?? []) {
      if (!seen.has(nb)) {
        seen.add(nb);
        queue.push(nb);
      }
    }
  }
  return {
    nodes: ns.filter((n) => seen.has(n.id)),
    edges: es.filter((e) => seen.has(e.source) && seen.has(e.target)),
  };
}

const viewGraph = computed<{ nodes: { id: string; label: string; kind: string }[]; edges: GraphEdge[] }>(() => {
  let ns = nodes.value;
  let es = edges.value;
  if (relationsOnly.value) {
    ({ nodes: ns, edges: es } = contractRelations(ns, es));
  }
  if (isolatedId.value && ns.some((n) => n.id === isolatedId.value)) {
    ({ nodes: ns, edges: es } = isolateAround(ns, es, isolatedId.value));
  }
  // Re-label the graph root as "focus" (a final pass, after contraction which
  // keeps only kind "root") so the viewed record reads in its own colour.
  const focus = rootNodeId.value;
  const out = ns.map((n) => ({ id: n.id, label: n.label, kind: n.id === focus ? "focus" : n.kind }));
  return { nodes: out, edges: es };
});

watch(relationsOnly, () => {
  isolatedId.value = null;
});

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
  isolatedId.value = null;
  rootNodeId.value = null;
  if (!props.record) return;
  // Backend level is 0-based: record=0, fields=1, rows=2.
  if (await fetchInto(props.record, level.value - 1)) {
    expanded.value.add(props.record);
    rootNodeId.value = nodes.value[0]?.id ?? null; // GraphFrom adds the root first
  }
}

function setLevel(l: number) {
  level.value = Math.min(3, Math.max(1, l));
  loadRoot();
}

async function onNodeClick(id: string) {
  // In relations-only mode a click isolates the record's connected paths (click
  // the same record again, or Show all, to clear). Otherwise it unfolds.
  if (relationsOnly.value) {
    isolatedId.value = isolatedId.value === id ? null : id;
    return;
  }
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
  <Modal
    :open="open"
    :title="t('datacore.graph_title')"
    width="860px"
    :dialog-style="{ height: '80vh' }"
    maximizable
    fill
    @close="emit('close')"
  >
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
        <button
          type="button"
          class="tool-btn"
          :class="{ 'is-active': relationsOnly }"
          :title="t('datacore.relations_only_hint')"
          @click="relationsOnly = !relationsOnly"
        >
          {{ t('datacore.relations_only') }}
        </button>
        <button
          v-if="relationsOnly && isolatedId"
          type="button"
          class="tool-btn"
          @click="isolatedId = null"
        >
          {{ t('datacore.show_all') }}
        </button>
        <span v-if="viewGraph.nodes.length" class="datacore-graph__count">
          {{ t('datacore.count', { nodes: viewGraph.nodes.length, edges: viewGraph.edges.length }) }}
        </span>
        <span class="datacore-graph__legend">
          <i class="datacore-dot datacore-dot--focus"></i>{{ t('datacore.legend_focus') }}
          <i class="datacore-dot datacore-dot--root"></i>{{ t('datacore.legend_root') }}
          <i class="datacore-dot datacore-dot--row"></i>{{ t('datacore.legend_row') }}
          <i class="datacore-dot datacore-dot--field"></i>{{ t('datacore.legend_field') }}
        </span>
      </div>

      <p v-if="!record" class="form-description">{{ t('datacore.no_record') }}</p>
      <p v-else-if="errorMsg" class="datacore-graph__error">{{ errorMsg }}</p>
      <p v-else-if="!nodes.length && loading" class="form-description">{{ t('datacore.loading') }}</p>
      <p v-else-if="!nodes.length" class="form-description">{{ t('datacore.empty') }}</p>
      <ForceGraph v-else :nodes="viewGraph.nodes" :edges="viewGraph.edges" @node-click="onNodeClick" />
    </div>
  </Modal>
</template>
