<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import { Codemirror } from "vue-codemirror";
import { EditorView } from "@codemirror/view";
import { oneDark } from "@codemirror/theme-one-dark";
import Modal from "../Modal.vue";
import SplitView from "../SplitView.vue";
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

// Compact-row status: the diagram's header line, or a muted "none".
const status = computed(() => {
  const first = value.value.split("\n").find((l) => l.trim()) ?? "";
  const trimmed = first.trim();
  if (!trimmed) return "";
  return trimmed.length > 60 ? `${trimmed.slice(0, 60)}…` : trimmed;
});
const modalTitle = computed(
  () => props.field.label || t("workspace.storage.field.mermaid.title"),
);

// mermaid.js is heavy and only needed when the dialog is open, so load it
// lazily and once. The live preview is client-side; export (markdown fence /
// server-baked SVG) is a separate pipeline.
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

const heightFill = EditorView.theme({
  "&": { height: "100%" },
  ".cm-scroller": { overflow: "auto" },
});
const extensions = computed(() => [
  ...(theme.value === "light" ? [] : [oneDark]),
  heightFill,
  EditorView.lineWrapping,
]);

// mermaid.render throws on a syntax error, so the preview pane doubles as
// the live validator: show the message instead of a broken diagram.
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
  } catch (e) {
    if (seq !== renderSeq) return;
    svg.value = "";
    errorMsg.value = e instanceof Error ? e.message : String(e);
  } finally {
    document.getElementById(id)?.remove();
    document.getElementById(`d${id}`)?.remove();
  }
}

// Render only while the dialog is open (open toggle, edits, theme switch).
let debounce: number | undefined;
watch([value, theme, open], () => {
  if (!open.value) return;
  window.clearTimeout(debounce);
  debounce = window.setTimeout(renderPreview, 250);
});
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
    <SplitView :initial="0.35" :min="0.15">
      <template #first>
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
      <template #second>
        <div class="mermaid-pane mermaid-pane--preview">
          <div v-if="errorMsg" class="mermaid-field-error">
            <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
            <pre>{{ errorMsg }}</pre>
          </div>
          <!-- mermaid renders with securityLevel "strict" (sanitised SVG). -->
          <!-- eslint-disable-next-line vue/no-v-html -->
          <div v-else-if="svg" class="mermaid-field-diagram" v-html="svg"></div>
          <div v-else class="mermaid-field-empty">
            {{ t('workspace.storage.field.mermaid.empty') }}
          </div>
        </div>
      </template>
    </SplitView>
  </Modal>
</template>
