<script setup lang="ts">
import { computed } from "vue";
import { SelectField, type SelectOption } from "../fields";
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

// Mirrors fieldFactory.resolveOption: each option may be a string or
// an object {value, label}. Empty options just give an empty list.
const options = computed<SelectOption[]>(() => {
  const raw = props.field.options ?? [];
  return raw.map((opt) => {
    if (typeof opt === "string") return { value: opt, label: opt };
    if (opt && typeof opt === "object") {
      const o = opt as Record<string, unknown>;
      const v = String(o.value ?? "");
      const l = String(o.label ?? o.value ?? "");
      return { value: v, label: l };
    }
    return { value: "", label: "" };
  });
});
</script>

<template>
  <SelectField v-model="value" :options="options" />
</template>
