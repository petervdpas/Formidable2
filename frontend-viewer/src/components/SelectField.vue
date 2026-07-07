<script setup lang="ts">
// Styled select, mirroring the main app's SelectField: a native <select> with
// appearance:none and a custom caret (see .field-select in styles.css), so it
// looks like the rest of the UI rather than the raw webview control.
export interface SelectOption {
  value: string;
  label: string;
}

const model = defineModel<string>({ default: "" });

defineProps<{
  options: SelectOption[];
  disabled?: boolean;
  id?: string;
}>();
</script>

<template>
  <select :id="id" :disabled="disabled" class="field-select" v-model="model">
    <option v-for="o in options" :key="o.value" :value="o.value">
      {{ o.label }}
    </option>
  </select>
</template>
