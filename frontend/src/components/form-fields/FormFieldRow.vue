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

