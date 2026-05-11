<script setup lang="ts">
import { computed } from "vue";
import { comboLabel } from "../utils/keyboardCombo";

const props = defineProps<{
  label: string;
  disabled?: boolean;
  /** Optional small text shown right-aligned (e.g. shortcut "Ctrl+Q"). */
  hint?: string;
  /** Keyboard combo (e.g. "Mod+S"). Rendered as the right-aligned hint
   * via comboLabel; supersedes the static `hint` prop when present. */
  combo?: string;
}>();

const emit = defineEmits<{ (e: "click"): void }>();

const displayHint = computed(() => {
  if (props.combo) return comboLabel(props.combo);
  return props.hint;
});

function onClick() {
  if (!props.disabled) emit("click");
}
</script>

<template>
  <button
    type="button"
    class="menu-item"
    role="menuitem"
    :disabled="disabled"
    @click="onClick"
  >
    <span class="menu-item-label">{{ label }}</span>
    <span v-if="displayHint" class="menu-item-hint">{{ displayHint }}</span>
  </button>
</template>
