<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ForceGraph from "./ForceGraph.vue";
import RenderedHtml from "./RenderedHtml.vue";
import {
  Service as DatacoreSvc,
  type GraphNode,
  type GraphEdge,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/datacore";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
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
// Default to the relation web: the full tensor draws every scalar value as its
// own node (a guid, "false", "int", a namespace), which is noise. Toggle off for
// the structural view, which still drops those value leaves (see dropScalarValues).
const relationsOnly = ref(true);
const isolatedId = ref<string | null>(null);
// The record the graph is rooted at (the one you opened); shown in a distinct
// colour so it stands out from the records it relates to.
const rootNodeId = ref<string | null>(null);
// Column labels per table field of the focused template, so a table tooltip can
// show a header row. Keyed by field key (the table container node's label).
const tableHeaders = ref<Record<string, string[]>>({});

async function loadHeaders() {
  tableHeaders.value = {};
  if (!props.templateFilename) return;
  try {
    const tpl = await TemplateSvc.LoadTemplate(props.templateFilename);
    const map: Record<string, string[]> = {};
    for (const f of tpl?.fields ?? []) {
      if (f.type !== "table") continue;
      const cols: string[] = [];
      for (const o of (f.options ?? []) as any[]) {
        const value = String(o?.value ?? "");
        if (!value) continue; // skip undefined columns (mirrors the loader)
        cols.push(String(o?.label ?? "") || value);
      }
      if (cols.length) map[f.key] = cols;
    }
    tableHeaders.value = map;
  } catch {
    tableHeaders.value = {};
  }
}
const REL_PREFIX = "rel:";
// Composite identities are "<template>\x1f<filename>" (datacore.NewID). The
// template prefix lets us tell a cross-template related record from a
// same-template one without a backend round-trip.
const ID_SEP = "\u001f";
function templateOf(id: string): string {
  return id.split(ID_SEP)[0];
}

// prettyField turns the internal relation key "rel:fcdm-entities.yaml" into a
// readable "fcdm-entities" for the structural-view node and edge labels; other
// labels pass through.
function prettyField(label: string): string {
  if (label.startsWith(REL_PREFIX)) {
    return label.slice(REL_PREFIX.length).replace(/\.yaml$/, "");
  }
  return label;
}

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

// graphInfo derives, from the FULL graph: the set of table-container field nodes
// (a field node with table-row children), and a per-node hover detail string. A
// record's detail lists its scalar values, table row counts, and relations; a
// table container lists its rows. Because it reads the full graph, the gated
// rows/values still surface on hover even though they're not drawn.
interface NodeTable {
  title: string;
  header?: string[]; // column labels, when known from the template
  rows: string[][]; // cell columns per row
  more: number; // rows beyond the cap, not shown
}
const TABLE_CAP = 16;
const graphInfo = computed<{
  containers: Set<string>;
  detail: Map<string, string>;
  tables: Map<string, NodeTable>;
}>(() => {
  const ns = nodes.value;
  const es = edges.value;
  const byId = new Map(ns.map((n) => [n.id, n]));
  const out = new Map<string, { target: string; field: string }[]>();
  for (const e of es) {
    const arr = out.get(e.source);
    if (arr) arr.push({ target: e.target, field: e.field });
    else out.set(e.source, [{ target: e.target, field: e.field }]);
  }
  const containers = new Set<string>();
  for (const n of ns) {
    if (n.kind === "field" && (out.get(n.id) ?? []).some((c) => byId.get(c.target)?.kind === "row")) {
      containers.add(n.id);
    }
  }
  const detail = new Map<string, string>();
  const tables = new Map<string, NodeTable>();
  for (const n of ns) {
    if (n.kind === "root") {
      const scalars: string[] = [];
      const tbls: string[] = [];
      const rels: string[] = [];
      for (const c of out.get(n.id) ?? []) {
        const f = byId.get(c.target);
        if (!f) continue;
        const fc = out.get(c.target) ?? [];
        if (c.field.startsWith(REL_PREFIX)) {
          rels.push(`→ ${prettyField(c.field)} (${fc.length})`);
        } else if (containers.has(c.target)) {
          tbls.push(`${c.field}: ${fc.length} rows`);
        } else if (fc.length === 0) {
          scalars.push(`${c.field}: ${f.label}`);
        }
      }
      detail.set(n.id, [n.label, ...scalars, ...tbls, ...rels].join("\n"));
    } else if (containers.has(n.id)) {
      const rowLabels = (out.get(n.id) ?? [])
        .map((c) => byId.get(c.target))
        .filter((r): r is GraphNode => !!r && r.kind === "row")
        .map((r) => r.label);
      tables.set(n.id, {
        title: `${n.label} (${rowLabels.length})`,
        header: tableHeaders.value[n.label], // n.label is the table field key
        rows: rowLabels.slice(0, TABLE_CAP).map((l) => l.split(" | ")),
        more: Math.max(0, rowLabels.length - TABLE_CAP),
      });
      detail.set(n.id, n.label);
    } else {
      detail.set(n.id, n.label);
    }
  }
  return { containers, detail, tables };
});

// gateStructural drops table ROWS and scalar-value leaves but keeps table
// CONTAINER nodes (one node per table) and relation field nodes, so the
// structural view shows a record's shape without exploding every row; the rows
// live in the container's hover tooltip.
function gateStructural(ns: GraphNode[], es: GraphEdge[], containers: Set<string>): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const drop = new Set<string>();
  for (const n of ns) {
    if (n.kind === "row") {
      drop.add(n.id);
    } else if (n.kind === "field" && !containers.has(n.id) && !n.label.startsWith(REL_PREFIX)) {
      drop.add(n.id);
    }
  }
  return {
    nodes: ns.filter((n) => !drop.has(n.id)),
    edges: es.filter((e) => !drop.has(e.source) && !drop.has(e.target)),
  };
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

const viewGraph = computed<{
  nodes: { id: string; label: string; kind: string; detail: string; table?: NodeTable }[];
  edges: { source: string; target: string; field: string }[];
}>(() => {
  const info = graphInfo.value;
  let ns = nodes.value;
  let es = edges.value;
  if (relationsOnly.value) {
    ({ nodes: ns, edges: es } = contractRelations(ns, es));
  } else {
    ({ nodes: ns, edges: es } = gateStructural(ns, es, info.containers));
  }
  if (isolatedId.value && ns.some((n) => n.id === isolatedId.value)) {
    ({ nodes: ns, edges: es } = isolateAround(ns, es, isolatedId.value));
  }
  // Final pass: mark the graph root as "focus" and cross-template records as
  // "related-cross" for colour, prettify relation labels, and attach the hover
  // detail (computed over the full graph so gated rows/values still show).
  const focus = rootNodeId.value;
  const focusTpl = focus ? templateOf(focus) : "";
  const out = ns.map((n) => {
    let kind = n.kind;
    if (n.id === focus) {
      kind = "focus";
    } else if (n.kind === "root" && focusTpl && templateOf(n.id) !== focusTpl) {
      kind = "related-cross"; // a record in another template
    }
    // Record nodes no longer show a hover tooltip: their content lives in the
    // inspector panel (click to open). Table/row/field nodes keep their tooltip.
    const isRecord = n.kind === "root";
    return {
      id: n.id,
      label: prettyField(n.label),
      kind,
      detail: isRecord ? "" : info.detail.get(n.id) ?? prettyField(n.label),
      table: isRecord ? undefined : info.tables.get(n.id),
    };
  });
  const outEdges = es.map((e) => ({ source: e.source, target: e.target, field: prettyField(e.field) }));
  return { nodes: out, edges: outEdges };
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
  closeInspector();
  void loadHeaders();
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
  const node = nodes.value.find((n) => n.id === id);
  // A record node (raw kind "root") opens the inspector with its rendered HTML,
  // in both modes. Field/row nodes have no HTML endpoint, so they don't.
  if (node?.kind === "root") void openInspector(id);

  // In relations-only mode a click isolates the record's connected paths (click
  // the same record again, or Show all, to clear). Otherwise it unfolds.
  if (relationsOnly.value) {
    isolatedId.value = isolatedId.value === id ? null : id;
    return;
  }
  // Field nodes are value leaves; only rows and linked records unfold further.
  if (!node || node.kind === "field") return;
  if (expanded.value.has(id)) return;
  expanded.value.add(id);
  await fetchInto(id, 2); // reveal the clicked node's fields and rows
}

// ── Inspector panel: rendered HTML of the clicked record ─────────────────
// Backend drives this through the STUDIO render service (the same path the HTML
// preview uses), so image URLs resolve in the webview and the prose carries the
// shared formidable-prose styling (tables, images). A record node's composite id
// is "<template>\x1f<datafile>" (datacore.NewID; the id part is always a
// datafile), which we split into RenderForm's two arguments.
const inspectorOpen = ref(false);
const inspectorFull = ref(false);
const inspectorLoading = ref(false);
const inspectorError = ref("");
const inspectorHtml = ref("");
const inspectorTitle = ref("");
const inspectorNodeId = ref<string | null>(null);

async function openInspector(id: string) {
  const parts = id.split(ID_SEP);
  if (parts.length < 2 || !parts[0] || !parts[1]) return;
  inspectorOpen.value = true;
  inspectorNodeId.value = id;
  inspectorLoading.value = true;
  inspectorError.value = "";
  inspectorHtml.value = "";
  // The panel title is the clicked record's graph label (its record title).
  inspectorTitle.value = nodes.value.find((n) => n.id === id)?.label ?? "";
  try {
    const res = await RenderSvc.RenderForm(parts[0], parts[1]);
    if (res?.html) {
      inspectorHtml.value = res.html;
    } else {
      inspectorError.value = t("datacore.inspector_not_found");
    }
  } catch (err) {
    inspectorError.value = backendErrMessage(err);
  } finally {
    inspectorLoading.value = false;
  }
}

function closeInspector() {
  inspectorOpen.value = false;
  inspectorFull.value = false;
  inspectorNodeId.value = null;
}

// Draggable divider between the graph and the inspector. inspectorRatio is the
// inspector's fraction of the body width; the graph takes the rest. Reuses the
// app-wide `.split-handle` look + `is-resizing-x` body cursor (same mechanics as
// SplitView, inlined here so the handle, graph, and inspector stay one markup
// tree across the open / split / full-screen states).
const bodyRef = ref<HTMLElement | null>(null);
const inspectorRatio = ref(0.42);
const inspectorPaneStyle = computed(() => ({
  flex: `0 0 ${(inspectorRatio.value * 100).toFixed(2)}%`,
}));

function clampInspector(r: number): number {
  return Math.max(0.2, Math.min(0.8, r));
}
function onInspectorDragMove(e: MouseEvent) {
  const el = bodyRef.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  if (rect.width <= 0) return;
  // Inspector is on the right, so its width is measured from the right edge.
  inspectorRatio.value = clampInspector((rect.right - e.clientX) / rect.width);
}
function onInspectorDragUp() {
  document.body.classList.remove("is-resizing-x");
  window.removeEventListener("mousemove", onInspectorDragMove);
  window.removeEventListener("mouseup", onInspectorDragUp);
}
function startInspectorDrag(e: MouseEvent) {
  document.body.classList.add("is-resizing-x");
  window.addEventListener("mousemove", onInspectorDragMove);
  window.addEventListener("mouseup", onInspectorDragUp);
  e.preventDefault();
}
function onInspectorHandleKey(e: KeyboardEvent) {
  const step = e.shiftKey ? 0.05 : 0.02;
  // ArrowLeft widens the inspector (it grows leftward); ArrowRight narrows it.
  if (e.key === "ArrowLeft") {
    inspectorRatio.value = clampInspector(inspectorRatio.value + step);
    e.preventDefault();
  }
  if (e.key === "ArrowRight") {
    inspectorRatio.value = clampInspector(inspectorRatio.value - step);
    e.preventDefault();
  }
}
onBeforeUnmount(onInspectorDragUp);

watch(
  () => props.open,
  (open) => {
    if (open) loadRoot();
    else {
      nodes.value = [];
      edges.value = [];
      closeInspector();
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
          <i class="datacore-dot datacore-dot--cross"></i>{{ t('datacore.legend_cross') }}
          <i class="datacore-dot datacore-dot--row"></i>{{ t('datacore.legend_row') }}
          <i class="datacore-dot datacore-dot--field"></i>{{ t('datacore.legend_field') }}
        </span>
      </div>

      <div ref="bodyRef" class="datacore-graph__body">
        <div v-show="!inspectorFull" class="datacore-graph__canvas">
          <p v-if="!record" class="form-description">{{ t('datacore.no_record') }}</p>
          <p v-else-if="errorMsg" class="datacore-graph__error">{{ errorMsg }}</p>
          <p v-else-if="!nodes.length && loading" class="form-description">{{ t('datacore.loading') }}</p>
          <p v-else-if="!nodes.length" class="form-description">{{ t('datacore.empty') }}</p>
          <ForceGraph v-else :nodes="viewGraph.nodes" :edges="viewGraph.edges" @node-click="onNodeClick" />
        </div>

        <div
          v-if="inspectorOpen && !inspectorFull"
          class="split-handle"
          role="separator"
          aria-orientation="vertical"
          tabindex="0"
          @mousedown="startInspectorDrag"
          @keydown="onInspectorHandleKey"
        ></div>

        <aside
          v-if="inspectorOpen"
          class="datacore-inspector"
          :class="{ 'datacore-inspector--full': inspectorFull }"
          :style="inspectorFull ? undefined : inspectorPaneStyle"
        >
          <header class="datacore-inspector__head">
            <span class="datacore-inspector__title">
              {{ inspectorTitle || t('datacore.inspector') }}
            </span>
            <button
              type="button"
              class="tool-btn"
              :title="inspectorFull ? t('datacore.inspector_collapse') : t('datacore.inspector_expand')"
              :aria-label="inspectorFull ? t('datacore.inspector_collapse') : t('datacore.inspector_expand')"
              @click="inspectorFull = !inspectorFull"
            >{{ inspectorFull ? '⤡' : '⤢' }}</button>
            <button
              type="button"
              class="tool-btn"
              :title="t('common.close')"
              :aria-label="t('common.close')"
              @click="closeInspector"
            >×</button>
          </header>
          <div class="datacore-inspector__body">
            <p v-if="inspectorLoading" class="form-description">{{ t('datacore.loading') }}</p>
            <p v-else-if="inspectorError" class="datacore-graph__error">{{ inspectorError }}</p>
            <RenderedHtml v-else class="datacore-inspector__html formidable-prose" :html="inspectorHtml" />
          </div>
        </aside>
      </div>
    </div>
  </Modal>
</template>
