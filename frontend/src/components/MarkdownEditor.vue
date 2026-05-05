<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useTemplateRef, watch } from "vue";
import EasyMDE from "easymde";
import "easymde/dist/easymde.min.css";
// EasyMDE's toolbar icons are FontAwesome glyphs. The library can
// auto-download from a CDN (autoDownloadFontAwesome) but we ship
// fully offline, so we import the local copy here.
import "@fortawesome/fontawesome-free/css/all.min.css";
import { useTheme } from "../composables/useTheme";

// MarkdownEditor wraps EasyMDE — same library and toolbar layout the
// original Formidable used. The composition is:
//
//   <textarea> ──new EasyMDE→ CodeMirror 5 editor with toolbar +
//                              status bar baked in.
//
// Two-way binding bridges Vue's defineModel and EasyMDE's value():
//   - external model change → setValue (guarded against the event
//     loop the change listener would otherwise close)
//   - editor change → emit update:modelValue
//
// Theme switch rebuilds the instance because EasyMDE doesn't support
// live theme swap on its underlying CodeMirror.

const props = withDefaults(
  defineProps<{
    /** Disable editing. */
    readonly?: boolean;
    /** Min editor height — passed to EasyMDE. */
    minHeight?: string;
  }>(),
  { readonly: false, minHeight: "120px" },
);

const model = defineModel<string>({ default: "" });

const { theme } = useTheme();

const textareaRef = useTemplateRef<HTMLTextAreaElement>("textarea");
const easymde = ref<EasyMDE | null>(null);

// Custom status bar items so the readout matches the original's
// "lines | words | characters | N Keystrokes" line.
let keystrokes = 0;

function makeStatus(): EasyMDE.Options["status"] {
  return [
    "lines",
    "words",
    {
      className: "characters",
      defaultValue: (el: HTMLElement) => {
        el.innerHTML = "characters: 0";
      },
      onUpdate: (el: HTMLElement) => {
        const text = easymde.value?.value() ?? "";
        el.innerHTML = `characters: ${text.length}`;
      },
    },
    {
      className: "keystrokes",
      defaultValue: (el: HTMLElement) => {
        el.innerHTML = "0 Keystrokes";
      },
      onUpdate: (el: HTMLElement) => {
        el.innerHTML = `${keystrokes} Keystrokes`;
      },
    },
  ];
}

// EasyMDE's "theme" option is the underlying CodeMirror 5 theme.
// monokai (dark) and eclipse (light) match the original.
function cmTheme(): string {
  return theme.value === "light" ? "eclipse" : "monokai";
}

let suppressEmit = false;

function build() {
  const el = textareaRef.value;
  if (!el) return;

  // Seed the textarea with current model so EasyMDE picks it up.
  el.value = model.value ?? "";

  const instance = new EasyMDE({
    element: el,
    initialValue: model.value ?? "",
    minHeight: props.minHeight,
    theme: cmTheme(),
    toolbar: [
      "bold",
      "italic",
      "strikethrough",
      "|",
      "quote",
      "unordered-list",
      "ordered-list",
      "|",
      "horizontal-rule",
      "code",
    ],
    status: makeStatus(),
    spellChecker: false,
    autoDownloadFontAwesome: false,
  });

  // Sync editor → model.
  const cm = instance.codemirror;
  cm.on("change", () => {
    if (suppressEmit) return;
    const next = instance.value();
    if (next !== model.value) model.value = next;
  });
  cm.on("keydown", () => {
    keystrokes += 1;
    // The status bar's onUpdate callbacks (defined in makeStatus)
    // fire on every cm change, so the keystroke counter reads the
    // latest value automatically. No explicit refresh needed.
  });

  // Readonly toggle is exposed on the underlying CM instance.
  if (props.readonly) cm.setOption("readOnly", true);

  easymde.value = instance;
}

function destroy() {
  const inst = easymde.value;
  if (!inst) return;
  // toTextArea unbinds CodeMirror and restores the original textarea
  // (which Vue then disposes via unmount).
  inst.toTextArea();
  easymde.value = null;
}

onMounted(build);
onBeforeUnmount(destroy);

// External model changes (Reset, programmatic load) flow into the
// editor without re-firing the editor's change handler.
watch(
  () => model.value,
  (v) => {
    const inst = easymde.value;
    if (!inst) return;
    if (inst.value() === (v ?? "")) return;
    suppressEmit = true;
    inst.value(v ?? "");
    suppressEmit = false;
  },
);

// Theme switch → rebuild. EasyMDE doesn't support live theme swap.
watch(theme, () => {
  destroy();
  // Wait one tick so the textarea ref re-attaches (we recreate it via
  // the v-if trigger on a key bound to theme).
  requestAnimationFrame(build);
});

// Readonly prop change — toggle on the underlying CodeMirror.
watch(
  () => props.readonly,
  (ro) => {
    easymde.value?.codemirror.setOption("readOnly", ro);
  },
);
</script>

<template>
  <div class="md-editor" :data-theme="theme">
    <!-- Re-keyed per theme so the textarea is freshly mounted when
         the theme changes — paired with build()/destroy() in the
         theme watcher. -->
    <textarea
      :key="theme"
      ref="textarea"
      class="md-textarea"
    ></textarea>
  </div>
</template>

