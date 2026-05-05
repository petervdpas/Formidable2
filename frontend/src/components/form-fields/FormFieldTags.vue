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

