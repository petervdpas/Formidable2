<script setup lang="ts">
/*
 * Popup — generic anchored popup with click-outside / escape close.
 *
 * Slots:
 *   - #trigger="{ toggle, open, close }" — required. Renders the
 *     button (or any element) that opens the popup. The slot props
 *     hand back the open state and the toggle/close functions so
 *     the trigger can render its own visual state (active border,
 *     etc.) without duplicating the open ref.
 *   - default slot — popup body. Receives `{ close }` so a button
 *     inside can dismiss the popup after acting.
 *
 * Positioning is anchored: the popup wrapper is `position: relative`
 * and the panel is `position: absolute` below the trigger. For
 * unconstrained positioning across overflow:hidden parents, callers
 * can pass `placement="below-right"` etc. — implemented as panel
 * classes; no JS-driven layout calc.
 */
import { onBeforeUnmount, ref, useTemplateRef, watch } from "vue";

const props = defineProps<{
  /** Where the panel opens relative to the trigger. Default "below". */
  placement?: "below" | "below-left" | "above" | "right" | "left";
  /** Optional max-width for the panel. */
  maxWidth?: string;
}>();

const open = ref(false);
const root = useTemplateRef<HTMLDivElement>("root");

function toggle() {
  open.value = !open.value;
}
function close() {
  open.value = false;
}

// Click-outside / escape close. Mirrors DropdownMenu's pattern so
// every popup in the app behaves the same way.
function onDocMouseDown(e: MouseEvent) {
  if (!open.value || !root.value) return;
  if (!root.value.contains(e.target as Node)) close();
}
function onKeydown(e: KeyboardEvent) {
  if (open.value && e.key === "Escape") {
    e.stopPropagation();
    close();
  }
}

watch(open, (isOpen) => {
  if (isOpen) {
    document.addEventListener("mousedown", onDocMouseDown, true);
    document.addEventListener("keydown", onKeydown, true);
  } else {
    document.removeEventListener("mousedown", onDocMouseDown, true);
    document.removeEventListener("keydown", onKeydown, true);
  }
});

onBeforeUnmount(() => {
  document.removeEventListener("mousedown", onDocMouseDown, true);
  document.removeEventListener("keydown", onKeydown, true);
});

const placementClass = () => `popup-${props.placement ?? "below"}`;
</script>

<template>
  <div ref="root" class="popup-wrap">
    <slot name="trigger" :toggle="toggle" :open="open" :close="close" />
    <div
      v-if="open"
      class="popup-panel"
      :class="placementClass()"
      :style="maxWidth ? { maxWidth } : undefined"
    >
      <slot :close="close" />
    </div>
  </div>
</template>
