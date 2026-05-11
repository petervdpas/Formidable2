<script setup lang="ts">
const props = defineProps<{
  label: string;
  disabled?: boolean;
  /** Optional small text shown right-aligned (e.g. shortcut "Ctrl+Q"). */
  hint?: string;
}>();

const emit = defineEmits<{ (e: "click"): void }>();

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
    <span v-if="hint" class="menu-item-hint">{{ hint }}</span>
  </button>
</template>
