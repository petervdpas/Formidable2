<script setup lang="ts">
import { computed } from "vue";
import { TextField } from "../fields";
import { Service as DialogSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/dialog";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Sibling of FormFieldFilePath; the picker accepts a directory
// instead of a file, and there are no per-extension filters.
//
// Same absolute-path normalization rule applies — picker output is
// already absolute, hand-typed input is coerced on focusout. Browse
// remains enabled when the field is read-only (read-only blocks
// free-typing but should not block picking a path).
const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const value = computed<string>({
  get: () => (props.modelValue == null ? "" : String(props.modelValue)),
  set: (v) => emit("update:modelValue", v),
});

async function coerceAbsolute() {
  if (!value.value) return;
  try {
    const abs = await SystemSvc.ResolveAbsolutePath(value.value);
    if (abs && abs !== value.value) value.value = abs;
  } catch {
    // Same fallback as FormFieldFilePath — typing the path stays valid.
  }
}

async function browse() {
  try {
    const picked = await DialogSvc.ChooseDirectory();
    if (picked) {
      value.value = picked;
      await coerceAbsolute();
    }
  } catch {
    // Same fallback as FormFieldFilePath — typing the path stays valid.
  }
}
</script>

<template>
  <div class="path-field" @focusout="coerceAbsolute">
    <TextField v-model="value" :readonly="field.readonly" />
    <button
      type="button"
      class="tool-btn path-field-browse"
      @click="browse"
    >
      …
    </button>
  </div>
</template>
