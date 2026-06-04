<script setup lang="ts">
import { computed } from "vue";
import { TextField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const value = computed<string>({
  get: () => (props.modelValue == null ? "" : String(props.modelValue)),
  set: (raw) => {
    const n = Number(raw);
    emit("update:modelValue", Number.isFinite(n) ? n : 0);
  },
});

// Step option ({value:"step", label:"1"|"any"|"<n>"}). Default "1"
// (integer-by-default); the author sets "any" for free decimals or e.g.
// "0.01" for currency. Mirrors range's option-driven step.
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
  return "1";
});
</script>

<template>
  <TextField type="number" :step="step" lazy v-model="value" :readonly="field.readonly" />
</template>
