<script setup lang="ts">
const model = defineModel<boolean>({ default: false });

defineProps<{
  onLabel?: string;
  offLabel?: string;
  disabled?: boolean;
  id?: string;
}>();
</script>

<template>
  <label class="field-switch">
    <input
      :id="id"
      type="checkbox"
      :disabled="disabled"
      v-model="model"
    />
    <span class="field-switch-track">
      <span class="field-switch-knob"></span>
    </span>
    <span v-if="onLabel || offLabel" class="field-switch-label">
      {{ model ? onLabel : offLabel }}
    </span>
  </label>
</template>
