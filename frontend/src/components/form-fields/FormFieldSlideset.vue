<script setup lang="ts">
import { computed } from "vue";
import { SelectField, type SelectOption } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

// A slideset picks one deck: the value is the chosen option's value.
const value = computed<string>({
  get: () => (props.modelValue == null ? "" : String(props.modelValue)),
  set: (v) => emit("update:modelValue", v),
});

// The authored decks (value/label), mirroring FormFieldDropdown's option parse.
const options = computed<SelectOption[]>(() => {
  const raw = props.field.options ?? [];
  return raw.map((opt) => {
    if (typeof opt === "string") return { value: opt, label: opt };
    if (opt && typeof opt === "object") {
      const o = opt as Record<string, unknown>;
      return { value: String(o.value ?? ""), label: String(o.label ?? o.value ?? "") };
    }
    return { value: "", label: "" };
  });
});
</script>

<template>
  <div class="slideset-widget">
    <i class="fa-solid fa-layer-group" aria-hidden="true"></i>
    <span class="slideset-widget-label">{{ field.label || field.key }}</span>
    <SelectField class="slideset-widget-input" v-model="value" :options="options" />
  </div>
</template>
