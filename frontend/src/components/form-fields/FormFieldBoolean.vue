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

// Resolve ON/OFF labels from field.options. Each option is the
// canonical {value, label} shape — the bool field type's fixed
// options shape (backend FixedOptionsShape) gives the user one row
// per state with value="true" / value="false" as the data and the
// label as the display string. We prefer a semantic match on value
// so order doesn't matter; fall back to position 0 / 1 when the
// values aren't the canonical strings (legacy data, or a bare
// "Yes|No" sub-option).
const labels = computed(() => {
  const opts = props.field.options ?? [];
  const labelOf = (o: unknown): string => {
    if (typeof o === "string") return o;
    if (o && typeof o === "object") {
      const rec = o as Record<string, unknown>;
      const l = rec.label;
      if (typeof l === "string" && l !== "") return l;
      const v = rec.value;
      if (typeof v === "string") return v;
    }
    return "";
  };
  const valueOf = (o: unknown): string => {
    if (typeof o === "string") return o;
    if (o && typeof o === "object") {
      const rec = o as Record<string, unknown>;
      const v = rec.value;
      if (typeof v === "string") return v;
    }
    return "";
  };

  let onLabel = "";
  let offLabel = "";
  for (const o of opts) {
    if (valueOf(o) === "true") onLabel = labelOf(o);
    if (valueOf(o) === "false") offLabel = labelOf(o);
  }
  if (!onLabel) onLabel = labelOf(opts[0]) || "On";
  if (!offLabel) offLabel = labelOf(opts[1]) || "Off";
  return { on: onLabel, off: offLabel };
});
</script>

<template>
  <SwitchField v-model="value" :on-label="labels.on" :off-label="labels.off" />
</template>
