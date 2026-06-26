<script setup lang="ts">
import { computed } from "vue";

// A single color input: a swatch that opens the OS color picker (a full wheel
// plus custom hex, so any color is reachable), a hex text field for exact
// values, and a clear button that resets to the empty/default state. The model
// is a #rrggbb string, or "" for "no color" (the consumer's default).
const props = withDefaults(
  defineProps<{
    modelValue: string;
    /** Seeds the OS picker when no color is set yet. */
    fallback?: string;
    /** Tooltip on the clear button. */
    clearTitle?: string;
    /** Tooltip on the swatch. */
    pickTitle?: string;
  }>(),
  { fallback: "#4a90e2", clearTitle: "Clear", pickTitle: "" },
);

const emit = defineEmits<{ (e: "update:modelValue", v: string): void }>();

const HEX6 = /^#[0-9a-fA-F]{6}$/;
const hasColor = computed(() => HEX6.test(props.modelValue.trim()));
// The native <input type=color> needs a concrete 7-char hex, so opening the
// picker on an unset field starts from the fallback rather than black.
const pickerValue = computed(() => (hasColor.value ? props.modelValue.trim() : props.fallback));

function onPick(e: Event) {
  emit("update:modelValue", (e.target as HTMLInputElement).value.toLowerCase());
}
function onHexChange(e: Event) {
  const el = e.target as HTMLInputElement;
  const raw = el.value.trim();
  if (raw === "") {
    emit("update:modelValue", "");
    return;
  }
  const hex = raw.startsWith("#") ? raw : "#" + raw;
  if (HEX6.test(hex)) emit("update:modelValue", hex.toLowerCase());
  else el.value = props.modelValue; // reject an invalid hex, restore the last good value
}
function clear() {
  emit("update:modelValue", "");
}
</script>

<template>
  <div class="color-field">
    <label
      class="color-field__swatch"
      :class="{ 'color-field__swatch--empty': !hasColor }"
      :style="hasColor ? { background: modelValue } : undefined"
      :title="pickTitle"
    >
      <input type="color" class="color-field__native" :value="pickerValue" @input="onPick" />
    </label>
    <input
      type="text"
      class="field-input color-field__hex"
      :value="modelValue"
      placeholder="#rrggbb"
      spellcheck="false"
      autocapitalize="off"
      autocomplete="off"
      @change="onHexChange"
    />
    <button
      v-if="hasColor"
      type="button"
      class="color-field__clear"
      :title="clearTitle"
      @click="clear"
    >×</button>
  </div>
</template>
