<script setup lang="ts">
import { computed } from "vue";
import { TextField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

// A sequence is an integer position: the value is always truncated to a whole
// number so a collection sorts cleanly by it.
const value = computed<string>({
  get: () => (props.modelValue == null ? "" : String(props.modelValue)),
  set: (raw) => {
    const n = Math.trunc(Number(raw));
    emit("update:modelValue", Number.isFinite(n) ? n : 0);
  },
});

// Step option ({value:"step", label:"10"}). Sparse default 10 so a row can be
// inserted between 10 and 20 without renumbering the rest.
const step = computed<string>(() => {
  for (const opt of props.field.options ?? []) {
    if (opt && typeof opt === "object") {
      const o = opt as Record<string, unknown>;
      if (String(o.value ?? "") === "step") {
        const s = String(o.label ?? "").trim();
        if (s !== "") return s;
      }
    }
  }
  return "10";
});
</script>

<template>
  <TextField type="number" :step="step" lazy v-model="value" :readonly="field.readonly" />
</template>
