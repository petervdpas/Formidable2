<script setup lang="ts">
// Styled on/off switch (button, not a native checkbox) so it matches the UI
// rather than the raw webview control.
const model = defineModel<boolean>({ default: false });
defineProps<{ disabled?: boolean }>();
</script>

<template>
  <button
    type="button"
    role="switch"
    class="switch"
    :class="{ on: model }"
    :aria-checked="model"
    :disabled="disabled"
    @click="model = !model"
  >
    <span class="switch-knob"></span>
  </button>
</template>
