<script setup lang="ts">
import { computed, ref } from "vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Minimal tags input — chips + add-by-Enter. Original supports
// drag-reorder, autocompletion against existing tags; patch later.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const tags = computed<string[]>({
  get: () => {
    const v = props.modelValue;
    if (Array.isArray(v)) return v.map(String);
    if (typeof v === "string") {
      return v.split(/[,;]/).map((s) => s.trim()).filter(Boolean);
    }
    return [];
  },
  set: (v) => emit("update:modelValue", v),
});

const draft = ref("");

function add() {
  const t = draft.value.trim().toLowerCase();
  if (!t) return;
  if (tags.value.includes(t)) {
    draft.value = "";
    return;
  }
  tags.value = [...tags.value, t];
  draft.value = "";
}

function remove(i: number) {
  tags.value = tags.value.filter((_, j) => j !== i);
}
</script>

<template>
  <div class="tags-field">
    <div v-if="tags.length" class="tag-row">
      <span v-for="(tag, i) in tags" :key="i" class="tag-chip">
        <span>{{ tag }}</span>
        <button
          v-if="!field.readonly"
          type="button"
          class="tag-remove"
          @click="remove(i)"
          aria-label="Remove tag"
        >×</button>
      </span>
    </div>
    <input
      v-if="!field.readonly"
      v-model="draft"
      class="field-input tag-input"
      placeholder="Add a tag — press Enter"
      @keydown.enter.prevent="add"
    />
  </div>
</template>

<style scoped>
.tags-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
}
.tag-row {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
}
.tag-chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 10px;
    border-radius: 999px;
    background: var(--color-surface-2);
    color: var(--color-text);
    font-size: var(--font-size-sm);
}
.tag-remove {
    appearance: none;
    background: transparent;
    border: 0;
    color: inherit;
    cursor: pointer;
    font-size: 14px;
    padding: 0 2px;
    line-height: 1;
}
.tag-remove:hover { color: var(--color-danger, #dc2626); }
</style>
