<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Codemirror } from "vue-codemirror";
import { EditorView, keymap } from "@codemirror/view";
import { Prec } from "@codemirror/state";
import { StreamLanguage } from "@codemirror/language";
import { markdown } from "@codemirror/lang-markdown";
import { yaml } from "@codemirror/lang-yaml";
import { lua as luaMode } from "@codemirror/legacy-modes/mode/lua";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "../composables/useTheme";

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    /** Language hint: 'markdown' (default — Handlebars/MD), 'yaml', 'lua'. */
    lang?: "markdown" | "yaml" | "lua";
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

// Lua uses a legacy stream parser (no first-class lang package
// exists) wrapped via StreamLanguage. yaml/markdown have proper
// LR parsers so we keep those on the dedicated packages.
const langExtension = computed(() => {
  if (props.lang === "yaml") return [yaml()];
  if (props.lang === "lua") return [StreamLanguage.define(luaMode)];
  return [markdown()];
});

// Ctrl+Enter / Cmd+Enter toggles full-screen mode — same gesture
// Formidable uses for "Template Code". Implemented as a CSS class on
// the root: fixed-position overlay over the whole webview.
const fullscreen = ref(false);

// Tidy: language-agnostic text cleanup. Normalizes line endings,
// strips trailing whitespace per line, collapses runs of >2 blank
// lines, and ensures exactly one trailing newline. Deliberately
// non-invasive — no syntactic re-indenting — so it's safe across
// every language CodeEditor handles (lua/markdown/yaml).
function tidy(src: string): string {
  let out = src.replace(/\r\n?/g, "\n");
  out = out
    .split("\n")
    .map((line) => line.replace(/[ \t]+$/, ""))
    .join("\n");
  out = out.replace(/\n{3,}/g, "\n\n");
  out = out.replace(/\n+$/, "") + "\n";
  return out;
}

function format() {
  const next = tidy(model.value);
  if (next !== model.value) {
    model.value = next;
  }
}

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
    // Shift+Alt+F mirrors VS Code's "Format Document" gesture.
    {
      key: "Shift-Alt-f",
      run: () => {
        format();
        return true;
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
    <div class="code-editor-toolbar">
      <button
        type="button"
        class="code-editor-action"
        :disabled="readonly"
        :title="t('codeeditor.format_title')"
        @click="format"
      >
        <i class="fa-solid fa-broom" aria-hidden="true"></i>
        <span>{{ t('codeeditor.format') }}</span>
      </button>
    </div>
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

