<script setup lang="ts">
import { computed } from "vue";
import { SwitchField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

// Boolean values may arrive as bool, "true"/"false" strings, or 0/1
// (legacy YAML round-trips). Normalize to bool for the switch.
const value = computed<boolean>({
  get: () => {
    const v = props.modelValue;
    if (typeof v === "boolean") return v;
    if (typeof v === "string") return v.toLowerCase() === "true";
    if (typeof v === "number") return v !== 0;
    return false;
  },
  set: (v) => emit("update:modelValue", v),
});

// Optional ON/OFF labels from field.options (matches original
// fieldFactory.boolean's "trailingValues" behaviour).
const labels = computed(() => {
  const opts = props.field.options ?? [];
  if (opts.length < 2) return { on: "On", off: "Off" };
  const norm = (o: unknown) =>
    typeof o === "string"
      ? o
      : o && typeof o === "object" && "label" in (o as Record<string, unknown>)
        ? String((o as Record<string, unknown>).label ?? "")
        : "";
  return { on: norm(opts[0]) || "On", off: norm(opts[1]) || "Off" };
});
</script>

<template>
  <SwitchField v-model="value" :on-label="labels.on" :off-label="labels.off" />
</template>
