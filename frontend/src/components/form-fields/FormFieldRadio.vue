<script setup lang="ts">
import { computed } from "vue";
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

const options = computed(() => {
  const raw = props.field.options ?? [];
  return raw.map((opt) => {
    if (typeof opt === "string") return { value: opt, label: opt };
    if (opt && typeof opt === "object") {
      const o = opt as Record<string, unknown>;
      return {
        value: String(o.value ?? ""),
        label: String(o.label ?? o.value ?? ""),
      };
    }
    return { value: "", label: "" };
  });
});
</script>

<template>
  <div class="radio-row">
    <label v-for="opt in options" :key="opt.value" class="radio-cell">
      <input
        type="radio"
        :name="field.key"
        :value="opt.value"
        :checked="value === opt.value"
        :disabled="field.readonly"
        @change="value = opt.value"
      />
      <span>{{ opt.label }}</span>
    </label>
  </div>
</template>

<style scoped>
.radio-row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
}
.radio-cell {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    cursor: pointer;
}
</style>
