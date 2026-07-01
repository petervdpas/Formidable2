<script setup lang="ts">
import { computed, nextTick, ref, watch, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import { Codemirror } from "vue-codemirror";
import { EditorView } from "@codemirror/view";
import { oneDark } from "@codemirror/theme-one-dark";
import Modal from "../Modal.vue";
import Tabs from "../Tabs.vue";
import type { TabItem } from "../Tabs.vue";
import { useTheme } from "../../composables/useTheme";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();
const { theme } = useTheme();

const value = computed<string>({
  get: () => (props.modelValue == null ? "" : String(props.modelValue)),
  set: (v) => emit("update:modelValue", v),
});

const open = ref(false);

// Edit and View are separate tabs, not split panes. With only one pane in the
// modal body at a time there is no splitter for the diagram's sizing to fight.
const activeTab = ref<"edit" | "view">("edit");
const tabItems = computed<TabItem[]>(() => [
  { id: "edit", label: t("workspace.storage.field.mermaid.tab_edit") },
  { id: "view", label: t("workspace.storage.field.mermaid.tab_view") },
]);

const status = computed(() => {
  const lines = value.value.split("\n").map((l) => l.trim());
  let body = lines;
  // Skip a leading YAML frontmatter block (--- … ---); prefer its title so the
  // status reads "Animal example" instead of the bare "---" delimiter.
  if (lines[0] === "---") {
    const end = lines.indexOf("---", 1);
    if (end > 0) {
      const titleLine = lines.slice(1, end).find((l) => l.startsWith("title:"));
      if (titleLine) {
        const title = titleLine.slice("title:".length).trim().replace(/^["']|["']$/g, "");
        if (title) return title.length > 60 ? `${title.slice(0, 60)}…` : title;
      }
      body = lines.slice(end + 1);
    }
  }
  const first = body.find((l) => l) ?? "";
  return first.length > 60 ? `${first.slice(0, 60)}…` : first;
});
const modalTitle = computed(
  () => props.field.label || t("workspace.storage.field.mermaid.title"),
);

type MermaidAPI = (typeof import("mermaid"))["default"];
let mermaidPromise: Promise<MermaidAPI> | null = null;
function loadMermaid(): Promise<MermaidAPI> {
  if (!mermaidPromise) mermaidPromise = import("mermaid").then((m) => m.default);
  return mermaidPromise;
}

const svg = ref("");
const errorMsg = ref("");
let renderSeq = 0;
let renderUid = 0;

// ── Isolated diagram viewer (iframe) ────────────────────────────────
// The SVG is painted into its own iframe document: a fully isolated view
// with its own scroll and its own coordinate space. App layout/CSS can't
// reach in and the diagram can't bleed out. Zoom is CSS `zoom` on a wrapper
// inside the frame; pan is the frame document's native scroll, also driven
// by drag. Zoom/pan/wheel handlers live inside the frame so the interaction
// is entirely self-contained.
const ZOOM_MIN = 0.25;
const ZOOM_MAX = 4;
const ZOOM_STEP = 1.2;
const frameEl = ref<HTMLIFrameElement | null>(null);
const zoom = ref(1);
const zoomPercent = computed(() => Math.round(zoom.value * 100));

function clampZoom(z: number): number {
  return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z));
}
function frameDoc(): Document | null {
  return frameEl.value?.contentDocument ?? null;
}
function frameWrap(): HTMLElement | null {
  return frameDoc()?.getElementById("mm-wrap") ?? null;
}
function applyZoom() {
  const wrap = frameWrap();
  if (wrap) wrap.style.zoom = String(zoom.value);
}
function zoomIn() {
  zoom.value = clampZoom(zoom.value * ZOOM_STEP);
  applyZoom();
}
function zoomOut() {
  zoom.value = clampZoom(zoom.value / ZOOM_STEP);
  applyZoom();
}
function zoomReset() {
  zoom.value = 1;
  applyZoom();
}

// Build the isolated frame's structure once (style + containers) by DOM
// injection - createElement/appendChild on its contentDocument, not
// document.write - and wire its self-contained pan/zoom interactions.
function ensureFrame(): Document | null {
  const doc = frameDoc();
  if (!doc || !doc.body) return null;
  if (doc.getElementById("mm-wrap")) return doc;

  const style = doc.createElement("style");
  style.textContent = `
    html, body { margin: 0; padding: 0; height: 100%; }
    body { overflow: auto; background: transparent; cursor: grab; }
    body.is-panning { cursor: grabbing; user-select: none; }
    #mm-pad { box-sizing: border-box; min-width: 100%; min-height: 100%;
      padding: 16px; display: flex; align-items: center; justify-content: center; }
    #mm-wrap svg { display: block; }
  `;
  doc.head.appendChild(style);

  const pad = doc.createElement("div");
  pad.id = "mm-pad";
  const wrap = doc.createElement("div");
  wrap.id = "mm-wrap";
  pad.appendChild(wrap);
  doc.body.appendChild(pad);

  wireFrame(doc);
  return doc;
}

// Self-contained interactions, bound to the frame's own document: Ctrl/Cmd +
// wheel zooms, drag pans via the frame's native scroll.
function wireFrame(doc: Document) {
  const scroller = (doc.scrollingElement ?? doc.documentElement) as HTMLElement;
  doc.addEventListener(
    "wheel",
    (e: WheelEvent) => {
      if (!(e.ctrlKey || e.metaKey)) return;
      e.preventDefault();
      zoom.value = clampZoom(zoom.value * (e.deltaY < 0 ? ZOOM_STEP : 1 / ZOOM_STEP));
      applyZoom();
    },
    { passive: false },
  );

  let dragging = false;
  let sx = 0;
  let sy = 0;
  let sl = 0;
  let st = 0;
  doc.addEventListener("pointerdown", (e: PointerEvent) => {
    if (e.button !== 0) return;
    dragging = true;
    sx = e.clientX;
    sy = e.clientY;
    sl = scroller.scrollLeft;
    st = scroller.scrollTop;
    doc.body.classList.add("is-panning");
  });
  doc.addEventListener("pointermove", (e: PointerEvent) => {
    if (!dragging) return;
    scroller.scrollLeft = sl - (e.clientX - sx);
    scroller.scrollTop = st - (e.clientY - sy);
  });
  const endDrag = () => {
    dragging = false;
    doc.body.classList.remove("is-panning");
  };
  doc.addEventListener("pointerup", endDrag);
  doc.addEventListener("pointerleave", endDrag);
}

// Inject the freshly-rendered SVG into the isolated frame (DOM injection via
// innerHTML on the wrapper) and apply the current zoom.
function paintFrame() {
  const doc = ensureFrame();
  const wrap = doc?.getElementById("mm-wrap");
  if (!wrap) return;
  wrap.innerHTML = svg.value;
  wrap.style.zoom = String(zoom.value);
}

const heightFill = EditorView.theme({
  "&": { height: "100%" },
  ".cm-scroller": { overflow: "auto" },
});
const extensions = computed(() => [
  ...(theme.value === "light" ? [] : [oneDark]),
  heightFill,
  EditorView.lineWrapping,
]);

// mermaid.render throws on a syntax error, so the preview doubles as the
// live validator: show the message instead of a broken diagram.
async function renderPreview() {
  const src = value.value.trim();
  const seq = ++renderSeq;
  if (!src) {
    svg.value = "";
    errorMsg.value = "";
    return;
  }
  const id = `mermaid-field-${++renderUid}`;
  try {
    const mermaid = await loadMermaid();
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: theme.value === "light" ? "default" : "dark",
    });
    const out = await mermaid.render(id, src);
    if (seq !== renderSeq) return;
    svg.value = out.svg;
    errorMsg.value = "";
    await nextTick();
    paintFrame();
  } catch (e) {
    if (seq !== renderSeq) return;
    svg.value = "";
    errorMsg.value = e instanceof Error ? e.message : String(e);
  } finally {
    document.getElementById(id)?.remove();
    document.getElementById(`d${id}`)?.remove();
  }
}

// Only render while the dialog is open and the View tab is showing: edits get
// debounced, switching to View (or opening) renders at once.
let debounce: number | undefined;
function scheduleRender(immediate: boolean) {
  if (!open.value || activeTab.value !== "view") return;
  window.clearTimeout(debounce);
  if (immediate) {
    void renderPreview();
    return;
  }
  debounce = window.setTimeout(renderPreview, 250);
}
// Open on the diagram when there's something to show, on the editor when empty.
// Reset zoom each time; this runs before the render watch below.
watch(open, (isOpen) => {
  if (!isOpen) return;
  zoom.value = 1;
  activeTab.value = value.value.trim() ? "view" : "edit";
});
watch([open, activeTab], () => scheduleRender(true));
watch([value, theme], () => scheduleRender(false));
onBeforeUnmount(() => window.clearTimeout(debounce));
</script>

<template>
  <div class="mermaid-trigger">
    <button type="button" class="tool-btn" @click="open = true">
      <i class="fa-solid fa-diagram-project" aria-hidden="true"></i>
      {{ t('workspace.storage.field.mermaid.open_editor') }}
    </button>
    <span class="mermaid-trigger-status" :class="{ muted: !status }">
      {{ status || t('workspace.storage.field.mermaid.no_diagram') }}
    </span>
  </div>

  <Modal
    :open="open"
    :title="modalTitle"
    width="960px"
    :dialog-style="{ height: '78vh' }"
    maximizable
    fill
    @close="open = false"
  >
    <div class="mermaid-tabs">
      <Tabs v-model="activeTab" :items="tabItems">
        <template #edit>
          <div class="mermaid-pane mermaid-pane--editor">
            <Codemirror
              v-model="value"
              :extensions="extensions"
              :disabled="field.readonly"
              :indent-with-tab="true"
              :placeholder="t('workspace.storage.field.mermaid.placeholder')"
            />
          </div>
        </template>
        <template #view>
          <div class="mermaid-pane mermaid-pane--preview">
          <div v-if="svg && !errorMsg" class="mermaid-zoom-toolbar">
            <button
              type="button"
              class="tool-btn"
              :title="t('workspace.storage.field.mermaid.zoom_out')"
              :aria-label="t('workspace.storage.field.mermaid.zoom_out')"
              @click="zoomOut"
            >
              <i class="fa-solid fa-magnifying-glass-minus" aria-hidden="true"></i>
            </button>
            <button
              type="button"
              class="tool-btn mermaid-zoom-level"
              :title="t('workspace.storage.field.mermaid.zoom_reset')"
              :aria-label="t('workspace.storage.field.mermaid.zoom_reset')"
              @click="zoomReset"
            >
              {{ zoomPercent }}%
            </button>
            <button
              type="button"
              class="tool-btn"
              :title="t('workspace.storage.field.mermaid.zoom_in')"
              :aria-label="t('workspace.storage.field.mermaid.zoom_in')"
              @click="zoomIn"
            >
              <i class="fa-solid fa-magnifying-glass-plus" aria-hidden="true"></i>
            </button>
          </div>
          <iframe
            v-show="svg && !errorMsg"
            ref="frameEl"
            class="mermaid-frame"
            :title="modalTitle"
          ></iframe>
          <div v-if="errorMsg" class="mermaid-field-error">
            <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
            <pre>{{ errorMsg }}</pre>
          </div>
          <div v-else-if="!svg" class="mermaid-field-empty">
            {{ t('workspace.storage.field.mermaid.empty') }}
          </div>
          </div>
        </template>
      </Tabs>
    </div>
  </Modal>
</template>
