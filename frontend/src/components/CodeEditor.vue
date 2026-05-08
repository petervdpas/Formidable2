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

// vue-codemirror doesn't expose its EditorView via the component
// ref — it emits a `ready` event with the view once mounted.
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
  return [markdown()];
});

// Ctrl+Enter / Cmd+Enter toggles full-screen mode — same gesture
// Formidable uses for "Template Code". Implemented as a CSS class on
// the root: fixed-position overlay over the whole webview.
const fullscreen = ref(false);

// Tidy: minimal text cleanup used as the formatter for non-Lua
// langs (markdown / yaml — touching their structure without a
// real parser is dangerous). Normalizes line endings, strips
// trailing whitespace, collapses runs of >2 blank lines, ensures
// exactly one trailing newline.
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

// Lua: real reformat via lua-fmt (luaparse-based). Lazy-imported
// so the ~100KB chunk only loads when the user clicks Format.
async function formatLua(src: string, indent: number): Promise<string> {
  const mod = await import("lua-fmt");
  return mod.formatText(src, {
    useTabs: false,
    indentCount: indent,
    lineWidth: 120,
    quotemark: "double",
    writeMode: mod.WriteMode.Replace,
  });
}

// Markdown (and our handlebars templates, which are just MD with
// {{...}} expressions left as text) via prettier standalone +
// markdown plugin. Lazy-imported. Tab size flows through to
// list-bullet indents. Handlebars blocks pass through verbatim
// because prettier's markdown parser treats {{...}} as inline
// text.
async function formatMarkdown(src: string, indent: number): Promise<string> {
  const [{ format }, mdPlugin] = await Promise.all([
    import("prettier/standalone"),
    import("prettier/plugins/markdown"),
  ]);
  return format(src, {
    parser: "markdown",
    plugins: [mdPlugin],
    tabWidth: indent,
    proseWrap: "preserve",
  });
}

async function format() {
  const view = editorView.value;
  if (!view) return;
  const cur = view.state.doc.toString();
  let next: string;
  try {
    if (props.lang === "lua") {
      next = await formatLua(cur, props.tabSize);
    } else if (props.lang === "markdown") {
      next = await formatMarkdown(cur, props.tabSize);
    } else {
      // YAML — no parser-based formatter wired; basic tidy keeps
      // the file syntactically intact (touching indent without a
      // YAML parser would be too risky).
      next = tidy(cur);
    }
  } catch {
    // Parse failure → fall back to basic tidy so the user at
    // least sees whitespace cleanup instead of nothing happening.
    next = tidy(cur);
  }
  if (next === cur) return;
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
      :placeholder="t('codeeditor.placeholder')"
      @ready="onReady"
    />
  </div>
</template>

