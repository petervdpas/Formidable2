<script setup lang="ts">
const model = defineModel<string>({ default: "" });

defineProps<{
  placeholder?: string;
  disabled?: boolean;
  readonly?: boolean;
  invalid?: boolean;
  id?: string;
  rows?: number;
}>();
</script>

<template>
  <textarea
    :id="id"
    :placeholder="placeholder"
    :disabled="disabled"
    :readonly="readonly"
    :rows="rows"
    :class="['field-textarea', { invalid }]"
    v-model="model"
  ></textarea>
</template>
