<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import ForceGraph from "./ForceGraph.vue";
import { api, type Graph, type GraphNode } from "../api";
import { reportError, viewerTheme } from "../state";

// The relations graph as Formidable does it: a bouncy force-directed web rooted
// at the record you opened it from (the focus hub, pinned centre). Fan out with
// hops; click a node to unfold its neighbours and load its HTML page beside the
// graph (draggable splitter). Positions are preserved as the web grows, so
// unfolding never reshuffles or blanks the layout.
const props = defineProps<{ bundleUrl: string; rootPage: string }>();
const emit = defineEmits<{ close: [] }>();

const root = ref<string>(""); // root record guid
const hops = ref(1);
const seeds = ref<Set<string>>(new Set()); // nodes the user unfolded
const notFound = ref(false);

const selectedTitle = ref("");
const selectedPage = ref("");
const detailSrc = computed(() => (selectedPage.value ? props.bundleUrl + selectedPage.value : ""));

// The detail iframe shows the record's page chrome-less (no top bar) in the
// user's theme. The page asks for its state on load; reply for our iframe only.
const detailEl = ref<HTMLIFrameElement | null>(null);
function onDetailMessage(e: MessageEvent): void {
  if (e.source !== detailEl.value?.contentWindow) return;
  const p = (e.data as { formidablePage?: unknown } | null)?.formidablePage;
  if (typeof p !== "string") return;
  (e.source as Window).postMessage({ viewerTheme: viewerTheme.value, bare: true }, "*");
}
watch(viewerTheme, (t) => {
  detailEl.value?.contentWindow?.postMessage({ viewerTheme: t, bare: true }, "*");
});

const byGuid = new Map<string, GraphNode>();
const adj = new Map<string, Set<string>>();
const full = ref<Graph>({ nodes: [], edges: [] });

const palette = ["#9b6bd4", "#3b82f6", "#10b981", "#ef4444", "#14b8a6", "#f59e0b", "#8b5cf6"];
const templateColor = new Map<string, string>();
function colorOf(template: string): string {
  if (!templateColor.has(template)) templateColor.set(template, palette[templateColor.size % palette.length]);
  return templateColor.get(template)!;
}

const rootTitle = computed(() => byGuid.get(root.value)?.title ?? "");

// Visible node set: the root fanned out `hops`, plus every unfolded seed and its
// direct neighbours.
const visible = computed<Set<string>>(() => {
  const seen = new Set<string>();
  if (!root.value) return seen;
  const expand = (start: string, depth: number) => {
    let frontier = [start];
    seen.add(start);
    for (let h = 0; h < depth; h++) {
      const next: string[] = [];
      for (const id of frontier) {
        for (const nb of adj.get(id) ?? []) {
          if (!seen.has(nb)) {
            seen.add(nb);
            next.push(nb);
          }
        }
      }
      frontier = next;
    }
  };
  expand(root.value, hops.value);
  for (const s of seeds.value) expand(s, 1);
  return seen;
});

const rootTpl = computed(() => byGuid.get(root.value)?.template ?? "");

const graphNodes = computed(() =>
  [...visible.value].map((guid) => {
    const n = byGuid.get(guid)!;
    // Label + colour mirror Formidable's Graph node settings: "<prefix>: <title>"
    // and the template's authored tint (falling back to a palette hue).
    const title = n.title || guid;
    const label = n.prefix ? `${n.prefix}: ${title}` : title;
    return {
      id: guid,
      label,
      detail: label,
      kind: guid === root.value ? "focus" : "related-cross",
      color: guid === root.value ? undefined : n.color || colorOf(n.template),
    };
  }),
);

const graphEdges = computed(() =>
  full.value.edges
    .filter((e) => visible.value.has(e.from) && visible.value.has(e.to))
    .map((e) => ({ source: e.from, target: e.to, field: "" })),
);

function onNodeClick(id: string): void {
  const n = byGuid.get(id);
  if (!n) return;
  seeds.value = new Set(seeds.value).add(id); // unfold its neighbours
  selectedTitle.value = n.title;
  selectedPage.value = n.page;
}

function stepHops(d: number): void {
  hops.value = Math.min(4, Math.max(1, hops.value + d));
}

onMounted(async () => {
  let g;
  try {
    g = await api.graph();
  } catch (e) {
    reportError(e);
    return;
  }
  full.value = g;
  for (const n of g.nodes) byGuid.set(n.guid, n);
  for (const e of g.edges) {
    (adj.get(e.from) ?? adj.set(e.from, new Set()).get(e.from)!).add(e.to);
    (adj.get(e.to) ?? adj.set(e.to, new Set()).get(e.to)!).add(e.from);
  }
  const start = g.nodes.find((n) => n.page === props.rootPage);
  if (!start) {
    notFound.value = true;
    return;
  }
  root.value = start.guid;
  selectedTitle.value = start.title;
  selectedPage.value = start.page;

  window.addEventListener("mousemove", onDrag);
  window.addEventListener("mouseup", endDrag);
  window.addEventListener("message", onDetailMessage);
});

onBeforeUnmount(() => {
  window.removeEventListener("mousemove", onDrag);
  window.removeEventListener("mouseup", endDrag);
  window.removeEventListener("message", onDetailMessage);
});

// --- splitter ---
const detailWidth = ref(520);
const body = ref<HTMLElement | null>(null);
let dragging = false;
function startDrag(): void {
  dragging = true;
  document.body.style.userSelect = "none";
}
function onDrag(e: MouseEvent): void {
  if (!dragging || !body.value) return;
  const rect = body.value.getBoundingClientRect();
  detailWidth.value = Math.min(rect.width - 280, Math.max(320, rect.right - e.clientX));
}
function endDrag(): void {
  dragging = false;
  document.body.style.userSelect = "";
}
</script>

<template>
  <div class="graph-view">
    <div class="graph-bar">
      <button class="btn ghost small" @click="emit('close')">{{ $t("graph.back") }}</button>
      <span v-if="!notFound" class="graph-ctl graph-root">{{ rootTitle }}</span>
      <span v-if="!notFound" class="graph-ctl">
        {{ $t("graph.hops") }}
        <button class="btn ghost small" @click="stepHops(-1)">−</button>
        <span class="graph-hops">{{ hops }}</span>
        <button class="btn ghost small" @click="stepHops(1)">+</button>
      </span>
    </div>

    <div ref="body" class="graph-body">
      <div v-if="notFound" class="graph-empty">{{ $t("graph.no_root") }}</div>
      <template v-else>
        <ForceGraph :nodes="graphNodes" :edges="graphEdges" @node-click="onNodeClick" />
        <div class="graph-splitter" @mousedown.prevent="startDrag"></div>
        <div class="graph-detail" :style="{ width: detailWidth + 'px' }">
          <div class="graph-detail-head">
            <strong>{{ selectedTitle || $t("graph.select_node") }}</strong>
          </div>
          <iframe v-if="detailSrc" ref="detailEl" :src="detailSrc" class="graph-detail-frame" title="record"></iframe>
          <div v-else class="graph-detail-empty">{{ $t("graph.select_node") }}</div>
        </div>
      </template>
    </div>
  </div>
</template>
