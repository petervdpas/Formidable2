<script setup lang="ts">
import { computed } from "vue";
import { MdEditor } from "md-editor-v3";
import "md-editor-v3/lib/style.css";
import { useTheme } from "../composables/useTheme";

// MarkdownEditor — Vue 3 native markdown editor built on CodeMirror 6
// via md-editor-v3. Native Vue ownership of the model and DOM lifecycle
// means no manual mount/teardown/refresh dance — `v-model` and the
// component handle their own measurement.

withDefaults(
  defineProps<{
    /** Disable editing. */
    readonly?: boolean;
  }>(),
  { readonly: false },
);

const model = defineModel<string>({ default: "" });

const { theme } = useTheme();

// md-editor-v3 ships two theme modes — 'light' / 'dark'. Map our
// three-way theme system: 'light' → light, anything else (dark,
// purplish) → dark. The component reads CSS variables underneath
// so any further token bridge happens in markdown-editor.css.
const editorTheme = computed<"light" | "dark">(() =>
  theme.value === "light" ? "light" : "dark",
);

// Toolbar — text formatting, quote, lists, inline + block code, link,
// table. md-editor-v3 separates groups with '-'. Items we don't need
// (image upload, prettier, preview, catalog, full-screen) are excluded
// so the bar stays compact.
const toolbars = [
  "bold",
  "italic",
  "strike-through",
  "-",
  "quote",
  "unordered-list",
  "ordered-list",
  "task",
  "-",
  "code-row",
  "code",
  "link",
  "table",
] as const;

// Footer = the status bar at the bottom. 'markdownTotal' shows total
// character count, '=' is a spacer, 'scrollSwitch' toggles editor /
// preview scroll sync (irrelevant here — preview is off — but cheap).
const footers = ["markdownTotal", "=", "scrollSwitch"] as const;
</script>

<template>
  <div class="md-editor-host" :data-theme="theme">
    <MdEditor
      v-model="model"
      :theme="editorTheme"
      :toolbars="toolbars as unknown as undefined"
      :footers="footers as unknown as undefined"
      :read-only="readonly"
      :preview="false"
      :no-upload-img="true"
      :no-mermaid="true"
      :no-katex="true"
      :no-prettier="true"
      language="en-US"
    />
  </div>
</template>
