<script setup lang="ts">
type InputType = "text" | "email" | "password" | "number" | "url" | "date";

const model = defineModel<string>({ default: "" });

const props = withDefaults(defineProps<{
  type?: InputType;
  placeholder?: string;
  disabled?: boolean;
  readonly?: boolean;
  invalid?: boolean;
  id?: string;
  autocomplete?: string;
  /** Forwarded to the underlying input for type="number" / "date". */
  min?: number | string;
  /** Forwarded to the underlying input for type="number" / "date". */
  max?: number | string;
  /** Forwarded to the underlying input for type="number". */
  step?: number | string;
  /** When true, render an X button while the value is non-empty. */
  clearable?: boolean;
}>(), {
  type: "text",
});

function clear() {
  model.value = "";
}

const showClear = () =>
  !!props.clearable && !props.disabled && !props.readonly && (model.value ?? "").length > 0;
</script>

<template>
  <span :class="['field-input-wrap', { clearable }]">
    <input
      :id="id"
      :type="type"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      :autocomplete="autocomplete"
      :min="min"
      :max="max"
      :step="step"
      :class="['field-input', { invalid }]"
      v-model="model"
    />
    <button
      v-if="showClear()"
      type="button"
      class="field-clear"
      :aria-label="'Clear'"
      tabindex="-1"
      @click="clear"
    >
      ×
    </button>
  </span>
</template>
