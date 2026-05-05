<script setup lang="ts">
import FormFieldRenderer from "./FormFieldRenderer.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

defineEmits<{ (e: "update:modelValue", v: unknown): void }>();
</script>

<template>
  <div :class="['form-field-row', { 'two-column': field.two_column }]">
    <div class="form-field-label-cell">
      <label class="form-field-label">{{ field.label || field.key }}</label>
      <p v-if="field.description" class="form-field-description">
        {{ field.description }}
      </p>
    </div>
    <div class="form-field-input-cell">
      <FormFieldRenderer
        :field="field"
        :model-value="modelValue"
        @update:model-value="(v: unknown) => $emit('update:modelValue', v)"
      />
    </div>
  </div>
</template>

<style scoped>
/* Default layout — label + description stacked above the input.
   Mirrors the original's "non-two_column" rendering. */
.form-field-row {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3) 0;
}

/* field.two_column → side-by-side: column 1 = label + description,
   column 2 = the actual edit control. */
.form-field-row.two-column {
    display: grid;
    grid-template-columns: minmax(180px, 28%) 1fr;
    gap: var(--space-3);
    align-items: start;
}

.form-field-row + .form-field-row {
    border-top: 1px dashed var(--color-border);
}
.form-field-label {
    font-weight: 600;
    color: var(--color-text);
}
.form-field-description {
    margin: 4px 0 0;
    font-size: var(--font-size-sm);
    color: var(--color-muted, #6b7280);
}
.form-field-input-cell {
    min-width: 0;
}
</style>
