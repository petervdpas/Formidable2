<script setup lang="ts">
import { computed, ref } from "vue";
import { Codemirror } from "vue-codemirror";
import { EditorView, keymap } from "@codemirror/view";
import { Prec } from "@codemirror/state";
import { markdown } from "@codemirror/lang-markdown";
import { yaml } from "@codemirror/lang-yaml";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "../composables/useTheme";

const props = withDefaults(
  defineProps<{
    /** Language hint: 'markdown' (default — Handlebars/MD), 'yaml'. */
    lang?: "markdown" | "yaml";
    /** Tab indentation size. Default 2. */
    tabSize?: number;
    /** Disable editing. */
    readonly?: boolean;
    /** Default editor height (px). User can drag the corner to resize. */
    height?: number;
  }>(),
  { lang: "markdown", tabSize: 2, readonly: false, height: 120 },
);

const model = defineModel<string>({ default: "" });

const { theme } = useTheme();

// One-Dark looks at home for dark/purplish; light theme uses CM6's
// default light styling (no extension).
const themeExtension = computed(() => (theme.value === "light" ? [] : [oneDark]));

const langExtension = computed(() => (props.lang === "yaml" ? [yaml()] : [markdown()]));

// Ctrl+Enter / Cmd+Enter toggles full-screen mode — same gesture
// Formidable uses for "Template Code". Implemented as a CSS class on
// the root: fixed-position overlay over the whole webview.
const fullscreen = ref(false);

const fullscreenKey = Prec.highest(
  keymap.of([
    {
      key: "Mod-Enter",
      run: () => {
        fullscreen.value = !fullscreen.value;
        return true;
      },
    },
    {
      key: "Escape",
      run: () => {
        if (fullscreen.value) {
          fullscreen.value = false;
          return true;
        }
        return false;
      },
    },
  ]),
);

// CodeMirror 6 grows to content unless you set a height via a theme
// extension. We anchor `&` (the editor root) to 100% of its host
// element — the wrapper's CSS height (which the user can drag) then
// becomes the visible editor height; longer content scrolls inside
// `.cm-scroller`.
const heightExtension = EditorView.theme({
  "&": { height: "100%" },
  ".cm-scroller": { overflow: "auto" },
});

const extensions = computed(() => [
  ...langExtension.value,
  ...themeExtension.value,
  heightExtension,
  fullscreenKey,
  EditorView.lineWrapping,
]);

// Wrapper height is the source of truth (it's what `resize: vertical`
// drags). Fullscreen mode uses the fixed-position overlay; the wrapper
// expands to inset:0 via the .fullscreen class so we don't override
// height inline in that case.
const wrapperStyle = computed(() =>
  fullscreen.value ? {} : { height: `${props.height}px` },
);
</script>

<template>
  <div
    :class="['code-editor', { fullscreen, readonly }]"
    :style="wrapperStyle"
  >
    <Codemirror
      v-model="model"
      :tab-size="tabSize"
      :extensions="extensions"
      :disabled="readonly"
      :indent-with-tab="true"
      placeholder="Type your template here. Ctrl+Enter to toggle full-screen."
    />
  </div>
</template>

<style scoped>
.code-editor {
    position: relative;
    border: 1px solid var(--input-border);
    border-radius: var(--radius-md);
    background: var(--input-bg);
    overflow: hidden;
    /* User can drag the corner to grow the editor; CodeMirror's
       theme extension binds & to height: 100% so it follows. */
    resize: vertical;
    min-height: 80px;
}

.code-editor.fullscreen {
    position: fixed;
    inset: 0;
    z-index: 1200;
    border: 0;
    border-radius: 0;
    resize: none;
    height: auto !important;
    width: auto !important;
}

.code-editor :deep(.cm-scroller) {
    font-family: var(--font-mono);
    font-size: 13px;
    line-height: 1.5;
}

.code-editor :deep(.cm-editor.cm-focused) {
    outline: none;
}

.code-editor:focus-within {
    border-color: var(--input-focus-border);
    box-shadow: 0 0 0 3px var(--input-focus-ring);
}
</style>
