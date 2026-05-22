<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Codemirror } from "vue-codemirror";
import { EditorView, keymap } from "@codemirror/view";
import { Prec } from "@codemirror/state";
import { StreamLanguage } from "@codemirror/language";
import { markdown } from "@codemirror/lang-markdown";
import { handlebarsMarkdownExtensions } from "./codemirror/handlebarsMarkdown";
import { yaml } from "@codemirror/lang-yaml";
import { html } from "@codemirror/lang-html";
import { lua as luaMode } from "@codemirror/legacy-modes/mode/lua";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "../composables/useTheme";
import { Service as CodeFormatterSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/codeformatter";

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    /** Language hint: 'markdown' (default - Handlebars/MD), 'yaml', 'lua', 'html'. */
    lang?: "markdown" | "yaml" | "lua" | "html";
    /** Tab indentation size. Default 2. */
    tabSize?: number;
    /** Disable editing. */
    readonly?: boolean;
    /** Default editor height (px). User can drag the corner to resize. */
    height?: number;
    /** Optional left-aligned toolbar caption - names the thing being
     *  edited so the full-screen overlay still shows context. */
    title?: string;
  }>(),
  { lang: "markdown", tabSize: 2, readonly: false, height: 120, title: "" },
);

const model = defineModel<string>({ default: "" });

// vue-codemirror doesn't expose its EditorView via the component
// ref - it emits a `ready` event with the view once mounted.
// Capture it there so format() can dispatch a transaction
// directly, avoiding any v-model round-trip staleness.
const editorView = ref<EditorView | null>(null);
function onReady(payload: { view: EditorView }) {
  editorView.value = payload.view;
}

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
  if (props.lang === "html") return [html()];
  return [markdown({ extensions: [handlebarsMarkdownExtensions] })];
});

// Ctrl+Enter / Cmd+Enter toggles full-screen mode - same gesture
// Formidable uses for "Template Code". Implemented as a CSS class on
// the root: fixed-position overlay over the whole webview.
const fullscreen = ref(false);

// Format delegates to the backend codeformatter service. The Go
// side owns the parser stack (yaml.v3 for YAML / markdown frontmatter,
// tidy pass for everything else) so paste artefacts in the webview
// can't shape the output. Errors surface as toasts via the catch
// block; the editor content is only replaced on success.
async function format() {
  const view = editorView.value;
  if (!view) return;
  const cur = view.state.doc.toString();
  let next: string;
  try {
    next = await CodeFormatterSvc.Format(props.lang, cur);
  } catch {
    return;
  }
  if (!next || next === cur) return;
  view.dispatch({
    changes: { from: 0, to: cur.length, insert: next },
  });
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
// element - the wrapper's CSS height (which the user can drag) then
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
      <span v-if="title" class="code-editor-title" :title="title">{{ title }}</span>
      <span class="code-editor-spacer"></span>
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
      :placeholder="t('codeeditor.placeholder')"
      @ready="onReady"
    />
  </div>
</template>

