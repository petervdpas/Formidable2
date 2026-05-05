<script setup lang="ts">
import { computed } from "vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const selected = computed<string[]>({
  get: () => {
    const v = props.modelValue;
    if (Array.isArray(v)) return v.map(String);
    return [];
  },
  set: (next) => emit("update:modelValue", next),
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

function toggle(value: string) {
  const cur = selected.value.slice();
  const i = cur.indexOf(value);
  if (i >= 0) cur.splice(i, 1);
  else cur.push(value);
  selected.value = cur;
}
</script>

<template>
  <div class="multioption">
    <label v-for="opt in options" :key="opt.value" class="multioption-row">
      <input
        type="checkbox"
        :checked="selected.includes(opt.value)"
        :disabled="field.readonly"
        @change="toggle(opt.value)"
      />
      <span>{{ opt.label }}</span>
    </label>
  </div>
</template>

<style scoped>
.multioption {
    display: flex;
    flex-direction: column;
    gap: 4px;
}
.multioption-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    cursor: pointer;
}
</style>
