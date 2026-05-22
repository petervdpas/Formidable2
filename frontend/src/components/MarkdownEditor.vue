<script setup lang="ts">
import { computed, ref } from "vue";
import { MdEditor, NormalToolbar } from "md-editor-v3";
import type { EditorView } from "@codemirror/view";
import "md-editor-v3/lib/style.css";
import { useTheme } from "../composables/useTheme";

// MarkdownEditor - Vue 3 native markdown editor built on CodeMirror 6
// via md-editor-v3. Native Vue ownership of the model and DOM lifecycle
// means no manual mount/teardown/refresh dance - `v-model` and the
// component handle their own measurement.
//
// Toolbar handlers are custom: md-editor-v3's defaults wrap the
// selection in markers on every click without checking whether the
// markers are already present, so a second click on already-bold text
// produces ****text**** instead of unwrapping. Three primitives
// (toggleInline / toggleLine / insertTemplate) + a button factory drive
// every visible button; adding a new one is one line in TOOLBAR_GROUPS.

withDefaults(
  defineProps<{
    /** Disable editing. */
    readonly?: boolean;
  }>(),
  { readonly: false },
);

const model = defineModel<string>({ default: "" });

const { theme } = useTheme();
const editorTheme = computed<"light" | "dark">(() =>
  theme.value === "light" ? "light" : "dark",
);

const editorRef = ref<{ getEditorView(): EditorView | undefined } | null>(
  null,
);

function withView(fn: (view: EditorView) => void) {
  const view = editorRef.value?.getEditorView();
  if (!view) return;
  fn(view);
  view.focus();
}

// ── Primitive 1: inline wrap toggle ────────────────────────────────
// Wrap selection in `open`/`close`. If the chars immediately outside
// the selection already match the markers, strip them instead. Works
// for single-char (italic *, inline code `), pairs (bold **, strike
// ~~), and multi-line block fences (```\n / \n```).
function toggleInline(open: string, close: string = open) {
  withView((view) => {
    const { state } = view;
    const sel = state.selection.main;
    const { doc } = state;
    const before = doc.sliceString(Math.max(0, sel.from - open.length), sel.from);
    const after = doc.sliceString(sel.to, sel.to + close.length);

    if (before === open && after === close) {
      view.dispatch({
        changes: [
          { from: sel.from - open.length, to: sel.from, insert: "" },
          { from: sel.to, to: sel.to + close.length, insert: "" },
        ],
        selection: {
          anchor: sel.from - open.length,
          head: sel.to - open.length,
        },
      });
      return;
    }

    const selected = doc.sliceString(sel.from, sel.to);
    view.dispatch({
      changes: { from: sel.from, to: sel.to, insert: `${open}${selected}${close}` },
      selection: {
        anchor: sel.from + open.length,
        head: sel.to + open.length,
      },
    });
  });
}

// ── Primitive 2: line-prefix toggle ────────────────────────────────
// For each line covered by the selection: if EVERY line already
// matches `targetRe` → strip; otherwise → set the target prefix
// (replacing any existing prefix matched by `stripRe` so ul→ol→task
// swaps cleanly). `prefix(idx)` lets ol number sequentially.
// `stripRe` left undefined for quote so quoting a list keeps the
// bullet ("> - foo").
interface LineSpec {
  prefix: (lineIdxInSelection: number) => string;
  targetRe: RegExp;
  stripRe?: RegExp;
}

function toggleLine(spec: LineSpec) {
  withView((view) => {
    const { state } = view;
    const sel = state.selection.main;
    const { doc } = state;
    const fromLine = doc.lineAt(sel.from);
    const toLine = doc.lineAt(sel.to);

    let allMatch = true;
    const lines: Array<{ from: number; text: string }> = [];
    for (let n = fromLine.number; n <= toLine.number; n++) {
      const line = doc.line(n);
      lines.push({ from: line.from, text: line.text });
      if (!spec.targetRe.test(line.text)) allMatch = false;
    }

    const changes: Array<{ from: number; to: number; insert: string }> = [];
    lines.forEach((line, idx) => {
      const indent = line.text.match(/^(\s*)/)?.[1] ?? "";

      if (allMatch) {
        const existing = line.text.match(spec.targetRe);
        const existingLen = existing ? existing[0].length : 0;
        changes.push({
          from: line.from,
          to: line.from + existingLen,
          insert: indent,
        });
      } else {
        const existing = spec.stripRe ? line.text.match(spec.stripRe) : null;
        const existingLen = existing ? existing[0].length : indent.length;
        changes.push({
          from: line.from,
          to: line.from + existingLen,
          insert: `${indent}${spec.prefix(idx)}`,
        });
      }
    });

    view.dispatch({ changes });
  });
}

// ── Primitive 3: composite insert ──────────────────────────────────
// For actions that produce a templated insert rather than a toggle
// (link, image, table). The builder receives the current selected
// text and returns the insert payload + the character range within
// the insert that should end up selected, so the user can type into
// the right slot (e.g. the URL of a link).
interface CompositeInsert {
  text: string;
  selectStart: number;
  selectEnd: number;
}

function insertTemplate(build: (selected: string) => CompositeInsert) {
  withView((view) => {
    const { state } = view;
    const sel = state.selection.main;
    const { doc } = state;
    const selected = doc.sliceString(sel.from, sel.to);
    const out = build(selected);

    view.dispatch({
      changes: { from: sel.from, to: sel.to, insert: out.text },
      selection: {
        anchor: sel.from + out.selectStart,
        head: sel.from + out.selectEnd,
      },
    });
  });
}

// ── Button factory ─────────────────────────────────────────────────
// Three discriminated-union variants matching the three primitives,
// produced by inlineBtn / lineBtn / templateBtn so the call site
// stays a single readable line per button.
type ButtonAction =
  | { kind: "inline"; open: string; close?: string }
  | { kind: "line"; spec: LineSpec }
  | { kind: "template"; build: (selected: string) => CompositeInsert };

interface ToolbarButton {
  id: string;
  title: string;
  icon: string;
  action: ButtonAction;
}

const inlineBtn = (id: string, title: string, icon: string, open: string, close?: string): ToolbarButton => ({
  id, title, icon, action: { kind: "inline", open, close },
});
const lineBtn = (id: string, title: string, icon: string, spec: LineSpec): ToolbarButton => ({
  id, title, icon, action: { kind: "line", spec },
});
const templateBtn = (
  id: string,
  title: string,
  icon: string,
  build: (selected: string) => CompositeInsert,
): ToolbarButton => ({
  id, title, icon, action: { kind: "template", build },
});

function runAction(action: ButtonAction) {
  if (action.kind === "inline") toggleInline(action.open, action.close);
  else if (action.kind === "line") toggleLine(action.spec);
  else insertTemplate(action.build);
}

// ── Action configs ─────────────────────────────────────────────────
// Shared "any-list-prefix" regex used by ul/ol/task as their stripRe.
// Quote intentionally doesn't reuse this - quoting a list keeps the bullet.
const ANY_LIST_PREFIX_RE = /^\s*(- \[[ xX]?\]\s|[*\-+]\s|\d+\.\s)/;

const LINE_SPEC = {
  quote: {
    prefix: () => "> ",
    targetRe: /^\s*> /,
  },
  ul: {
    prefix: () => "- ",
    targetRe: /^\s*[*\-+]\s(?!\[)/,
    stripRe: ANY_LIST_PREFIX_RE,
  },
  ol: {
    prefix: (i: number) => `${i + 1}. `,
    targetRe: /^\s*\d+\.\s/,
    stripRe: ANY_LIST_PREFIX_RE,
  },
  task: {
    prefix: () => "- [ ] ",
    targetRe: /^\s*- \[[ xX]?\]\s/,
    stripRe: ANY_LIST_PREFIX_RE,
  },
} satisfies Record<string, LineSpec>;

// Link template: insert `[text](url)`. If there's a selection it
// becomes the text. The dispatch leaves the literal "url" highlighted
// so the next keystroke replaces it.
function linkBuild(selected: string): CompositeInsert {
  const text = selected || "text";
  const inserted = `[${text}](url)`;
  return {
    text: inserted,
    selectStart: inserted.length - 4,
    selectEnd: inserted.length - 1,
  };
}

// Table template: minimal 2x3 table. Selection becomes the first header cell.
function tableBuild(selected: string): CompositeInsert {
  const headCell = selected || "Header";
  const tbl =
    `| ${headCell} | Header | Header |\n` +
    `| --- | --- | --- |\n` +
    `| Cell | Cell | Cell |`;
  return { text: tbl, selectStart: 2, selectEnd: 2 + headCell.length };
}

// ── Toolbar layout ─────────────────────────────────────────────────
// Groups of buttons; flattened into the `defToolbars` slot in render
// order, with separators inserted between groups in the `toolbars`
// prop. Add a new button by dropping it in the right group - no other
// changes needed.
const TOOLBAR_GROUPS: ToolbarButton[][] = [
  [
    inlineBtn("bold", "Bold", "fa-bold", "**"),
    inlineBtn("italic", "Italic", "fa-italic", "*"),
    inlineBtn("strike", "Strikethrough", "fa-strikethrough", "~~"),
  ],
  [
    lineBtn("quote", "Quote", "fa-quote-right", LINE_SPEC.quote),
    lineBtn("ul", "Unordered list", "fa-list-ul", LINE_SPEC.ul),
    lineBtn("ol", "Ordered list", "fa-list-ol", LINE_SPEC.ol),
    lineBtn("task", "Task list", "fa-list-check", LINE_SPEC.task),
  ],
  [
    inlineBtn("inline-code", "Inline code", "fa-code", "`"),
    inlineBtn("code-block", "Code block", "fa-file-code", "```\n", "\n```"),
  ],
  [
    templateBtn("link", "Link", "fa-link", linkBuild),
    templateBtn("table", "Table", "fa-table", tableBuild),
  ],
];

// Flattened view for the v-for in the template.
const BUTTONS: ToolbarButton[] = TOOLBAR_GROUPS.flat();

// `toolbars` prop for MdEditor: numbers index defToolbars children;
// '-' is a separator. Built from the groups + separators between them.
const toolbars = computed(() => {
  const out: Array<number | "-"> = [];
  let idx = 0;
  TOOLBAR_GROUPS.forEach((group, gi) => {
    if (gi > 0) out.push("-");
    for (let _ = 0; _ < group.length; _++) out.push(idx++);
  });
  return out;
});

// Every built-in we replace with a custom toggle/template version.
const toolbarsExclude = [
  "bold",
  "italic",
  "strike-through",
  "quote",
  "unordered-list",
  "ordered-list",
  "task",
  "code-row",
  "code",
  "link",
  "table",
] as const;

const footers = ["markdownTotal", "=", "scrollSwitch"] as const;
</script>

<template>
  <div class="md-editor-host" :data-theme="theme">
    <MdEditor
      ref="editorRef"
      v-model="model"
      :theme="editorTheme"
      :toolbars="toolbars as unknown as undefined"
      :toolbars-exclude="toolbarsExclude as unknown as undefined"
      :footers="footers as unknown as undefined"
      :read-only="readonly"
      :preview="false"
      :no-upload-img="true"
      :no-mermaid="true"
      :no-katex="true"
      :no-prettier="true"
      language="en-US"
    >
      <template #defToolbars>
        <NormalToolbar
          v-for="btn in BUTTONS"
          :key="btn.id"
          :title="btn.title"
          @on-click="runAction(btn.action)"
        >
          <template #trigger>
            <i :class="['fa-solid', btn.icon]" aria-hidden="true"></i>
          </template>
        </NormalToolbar>
      </template>
    </MdEditor>
  </div>
</template>
