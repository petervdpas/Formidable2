<script setup lang="ts">
import SwitchField from "./SwitchField.vue";

// FormSwitchRow - purpose-built FormSection row for boolean
// switches. Unlike a generic FormRow that stacks the description
// inside form-control (and pushes a short input visually upward
// when centred), this lays out:
//   row 1: label | switch     ← single line, vertically centred
//   row 2: (blank) | description (full-width muted text)
// so the toggle stays visually anchored to the label and the
// description doesn't compete with it for the center line.
const model = defineModel<boolean>({ default: false });

defineProps<{
  label?: string;
  description?: string;
  onLabel?: string;
  offLabel?: string;
  disabled?: boolean;
  id?: string;
  // Forwarded to SwitchField: opt into the controlled (:checked) checkbox.
  controlled?: boolean;
}>();
</script>

<template>
  <div class="form-switch-row">
    <label v-if="label" class="form-label" :for="id">{{ label }}</label>
    <div v-else></div>
    <div class="form-switch-row-control">
      <SwitchField
        v-model="model"
        :on-label="onLabel"
        :off-label="offLabel"
        :disabled="disabled"
        :id="id"
        :controlled="controlled"
      />
    </div>
    <p v-if="description" class="form-switch-row-description">
      {{ description }}
    </p>
  </div>
</template>
