<script setup lang="ts">
import { computed } from "vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

// Range options are {value:"min"|"max"|"step", label:"<number>"} pairs
// (matches the original fieldFactory.range behaviour). Pull them out
// with sensible defaults.
const settings = computed(() => {
  const map: Record<string, number> = { min: 0, max: 100, step: 1 };
  for (const opt of props.field.options ?? []) {
    if (!opt || typeof opt !== "object") continue;
    const o = opt as Record<string, unknown>;
    const k = String(o.value ?? "");
    const n = Number(o.label ?? o.value);
    if (k in map && Number.isFinite(n)) map[k] = n;
  }
  return map;
});

const value = computed<number>({
  get: () => {
    const v = Number(props.modelValue ?? settings.value.min);
    return Number.isFinite(v) ? v : settings.value.min;
  },
  set: (v) => emit("update:modelValue", v),
});
</script>

<template>
  <div class="range-field">
    <input
      type="range"
      :min="settings.min"
      :max="settings.max"
      :step="settings.step"
      :readonly="field.readonly"
      v-model.number="value"
    />
    <span class="range-display">{{ value }}</span>
  </div>
</template>

<style scoped>
.range-field {
    display: flex;
    align-items: center;
    gap: var(--space-2);
}
.range-field input[type="range"] {
    flex: 1 1 auto;
}
.range-display {
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
    min-width: 3ch;
    text-align: right;
}
</style>
