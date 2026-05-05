<script setup lang="ts">
defineProps<{
  label?: string;
  description?: string;
  error?: string;
  /** Optional: id of the control inside, so the label points to it. */
  labelFor?: string;
}>();
</script>

<template>
  <div class="form-row">
    <label v-if="label" class="form-label" :for="labelFor">{{ label }}</label>
    <div v-else></div>
    <div class="form-control">
      <slot />
      <p v-if="description" class="form-description">{{ description }}</p>
      <p v-if="error" class="form-error">{{ error }}</p>
    </div>
  </div>
</template>
