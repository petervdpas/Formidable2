<script setup lang="ts">
import { ref, watch, onBeforeUnmount, useTemplateRef } from "vue";

const props = defineProps<{
  /** Visible label of the menu button (e.g. "File"). */
  label: string;
  /** When true, the button can't be opened and any open popup closes. */
  disabled?: boolean;
}>();

const open = ref(false);
const rootRef = useTemplateRef<HTMLDivElement>("root");

function toggle() {
  if (props.disabled) return;
  open.value = !open.value;
}
function close() { open.value = false; }

function onDocumentMouseDown(e: MouseEvent) {
  if (!open.value || !rootRef.value) return;
  if (!rootRef.value.contains(e.target as Node)) close();
}

function onKeydown(e: KeyboardEvent) {
  if (open.value && e.key === "Escape") {
    e.stopPropagation();
    close();
  }
}

// Close immediately if the button becomes disabled while open.
watch(() => props.disabled, (d) => {
  if (d) close();
});

watch(open, (isOpen) => {
  if (isOpen) {
    document.addEventListener("mousedown", onDocumentMouseDown, true);
    document.addEventListener("keydown", onKeydown, true);
  } else {
    document.removeEventListener("mousedown", onDocumentMouseDown, true);
    document.removeEventListener("keydown", onKeydown, true);
  }
});

onBeforeUnmount(() => {
  document.removeEventListener("mousedown", onDocumentMouseDown, true);
  document.removeEventListener("keydown", onKeydown, true);
});
</script>

<template>
  <div ref="root" class="dropdown-menu" :class="{ open }">
    <button
      type="button"
      class="topmenu-item"
      :disabled="disabled"
      :aria-haspopup="true"
      :aria-expanded="open"
      @click="toggle"
    >
      {{ label }}
    </button>
    <div
      v-if="open && !disabled"
      class="dropdown-popup"
      role="menu"
      @click="close"
    >
      <slot />
    </div>
  </div>
</template>
