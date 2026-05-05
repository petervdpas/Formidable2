<script setup lang="ts">
type InputType = "text" | "email" | "password" | "number" | "url" | "date";

const model = defineModel<string>({ default: "" });

withDefaults(defineProps<{
  type?: InputType;
  placeholder?: string;
  disabled?: boolean;
  readonly?: boolean;
  invalid?: boolean;
  id?: string;
  autocomplete?: string;
}>(), {
  type: "text",
});
</script>

<template>
  <input
    :id="id"
    :type="type"
    :placeholder="placeholder"
    :disabled="disabled"
    :readonly="readonly"
    :autocomplete="autocomplete"
    :class="['field-input', { invalid }]"
    v-model="model"
  />
</template>
