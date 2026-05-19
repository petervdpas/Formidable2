<script setup lang="ts">
import { computed } from "vue";
import { TextareaField } from "../fields";
import MarkdownEditor from "../MarkdownEditor.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const value = computed<string>({
  get: () => (props.modelValue == null ? "" : String(props.modelValue)),
  set: (v) => emit("update:modelValue", v),
});

// "markdown" → MarkdownEditor (toolbar + footer, CodeMirror 6-backed
// via md-editor-v3). "plain" → plain textarea.
const isMarkdown = computed(() => (props.field.format ?? "markdown") === "markdown");
</script>

<template>
  <MarkdownEditor
    v-if="isMarkdown"
    v-model="value"
    :readonly="field.readonly"
  />
  <TextareaField
    v-else
    v-model="value"
    :readonly="field.readonly"
    :rows="6"
  />
</template>
