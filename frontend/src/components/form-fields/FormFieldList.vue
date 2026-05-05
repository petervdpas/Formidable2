<script setup lang="ts">
import { computed } from "vue";
import { TextField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Minimal list editor — Add/Remove + per-row text input. The original
// supports drag-reorder + paste-as-rows; we patch those in later.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const items = computed<string[]>({
  get: () => {
    const v = props.modelValue;
    if (Array.isArray(v)) return v.map(String);
    return [];
  },
  set: (v) => emit("update:modelValue", v),
});

function setItem(i: number, value: string) {
  const next = items.value.slice();
  next[i] = value;
  items.value = next;
}

function add() {
  items.value = [...items.value, ""];
}

function remove(i: number) {
  items.value = items.value.filter((_, j) => j !== i);
}
</script>

<template>
  <div class="list-field">
    <div v-for="(item, i) in items" :key="i" class="list-row">
      <TextField
        :model-value="item"
        @update:model-value="(v) => setItem(i, v)"
        :readonly="field.readonly"
      />
      <button
        v-if="!field.readonly"
        type="button"
        class="list-btn remove"
        @click="remove(i)"
        aria-label="Remove item"
      >−</button>
    </div>
    <button
      v-if="!field.readonly"
      type="button"
      class="list-btn add"
      @click="add"
    >+</button>
  </div>
</template>

<style scoped>
.list-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
}
.list-row {
    display: flex;
    gap: 6px;
    align-items: center;
}
.list-row :deep(.field-input) {
    flex: 1 1 auto;
}
.list-btn {
    flex: 0 0 auto;
    width: 32px;
    height: 34px;
    appearance: none;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
    color: var(--color-text);
    border-radius: var(--radius-md);
    cursor: pointer;
    font-size: 16px;
    line-height: 1;
    font-weight: 600;
}
.list-btn:hover { background: var(--color-surface-2); }
.list-btn.add { width: 100%; }
</style>
