<script setup lang="ts">
const model = defineModel<boolean>({ default: false });

defineProps<{
  label?: string;
  disabled?: boolean;
  id?: string;
}>();
</script>

<template>
  <label class="field-checkbox">
    <input
      :id="id"
      type="checkbox"
      :disabled="disabled"
      v-model="model"
    />
    <span v-if="label">{{ label }}</span>
  </label>
</template>
