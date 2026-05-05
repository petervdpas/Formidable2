<script setup lang="ts">
export type SelectOption = string | { value: string; label: string };

const model = defineModel<string>({ default: "" });

defineProps<{
  options: SelectOption[];
  disabled?: boolean;
  invalid?: boolean;
  id?: string;
  placeholder?: string;
}>();

function optValue(o: SelectOption): string {
  return typeof o === "string" ? o : o.value;
}
function optLabel(o: SelectOption): string {
  return typeof o === "string" ? o : o.label;
}
</script>

<template>
  <select
    :id="id"
    :disabled="disabled"
    :class="['field-select', { invalid }]"
    v-model="model"
  >
    <option v-if="placeholder" value="" disabled>{{ placeholder }}</option>
    <option v-for="o in options" :key="optValue(o)" :value="optValue(o)">
      {{ optLabel(o) }}
    </option>
  </select>
</template>
