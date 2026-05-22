<script setup lang="ts">
/*
 * Popup - generic anchored popup with click-outside / escape close.
 *
 * Slots:
 *   - #trigger="{ toggle, open, close }" - required. Renders the
 *     button (or any element) that opens the popup. The slot props
 *     hand back the open state and the toggle/close functions so
 *     the trigger can render its own visual state (active border,
 *     etc.) without duplicating the open ref.
 *   - default slot - popup body. Receives `{ close }` so a button
 *     inside can dismiss the popup after acting.
 *
 * Positioning is anchored: the popup wrapper is `position: relative`
 * and the panel is `position: absolute` below the trigger. For
 * unconstrained positioning across overflow:hidden parents, callers
 * can pass `teleport` to render the panel into <body> with
 * computed fixed coordinates - bypasses any clipping ancestor.
 */
import { computed, nextTick, onBeforeUnmount, ref, useTemplateRef, watch } from "vue";

const props = defineProps<{
  /** Where the panel opens relative to the trigger. Default "below". */
  placement?: "below" | "below-left" | "above" | "right" | "left";
  /** Optional max-width for the panel. */
  maxWidth?: string;
  /** Render the panel into <body> with position:fixed coords so it
   *  escapes any overflow:hidden / overflow:auto ancestor (e.g. a
   *  modal-dialog or a scrollable list). */
  teleport?: boolean;
}>();

const open = ref(false);
const root = useTemplateRef<HTMLDivElement>("root");
const fixedStyle = ref<Record<string, string>>({});

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
  const target = e.target as Node;
  if (root.value.contains(target)) return;
  // Teleported panels live outside root.value; check them too.
  const panels = document.querySelectorAll('[data-popup-panel="1"]');
  for (const p of panels) {
    if (p.contains(target)) return;
  }
  close();
}
function onKeydown(e: KeyboardEvent) {
  if (open.value && e.key === "Escape") {
    e.stopPropagation();
    close();
  }
}

function recomputeFixedCoords() {
  if (!props.teleport || !open.value || !root.value) return;
  const rect = root.value.getBoundingClientRect();
  const gap = 4;
  const placement = props.placement ?? "below";
  const style: Record<string, string> = { position: "fixed" };
  switch (placement) {
    case "below":
      style.top = `${rect.bottom + gap}px`;
      style.left = `${rect.left}px`;
      break;
    case "below-left":
      style.top = `${rect.bottom + gap}px`;
      style.right = `${window.innerWidth - rect.right}px`;
      break;
    case "above":
      style.bottom = `${window.innerHeight - rect.top + gap}px`;
      style.left = `${rect.left}px`;
      break;
    case "right":
      style.top = `${rect.top}px`;
      style.left = `${rect.right + gap}px`;
      break;
    case "left":
      style.top = `${rect.top}px`;
      style.right = `${window.innerWidth - rect.left + gap}px`;
      break;
  }
  fixedStyle.value = style;
}

function onScrollOrResize() {
  recomputeFixedCoords();
}

watch(open, async (isOpen) => {
  if (isOpen) {
    document.addEventListener("mousedown", onDocMouseDown, true);
    document.addEventListener("keydown", onKeydown, true);
    if (props.teleport) {
      window.addEventListener("scroll", onScrollOrResize, true);
      window.addEventListener("resize", onScrollOrResize);
      await nextTick();
      recomputeFixedCoords();
    }
  } else {
    document.removeEventListener("mousedown", onDocMouseDown, true);
    document.removeEventListener("keydown", onKeydown, true);
    window.removeEventListener("scroll", onScrollOrResize, true);
    window.removeEventListener("resize", onScrollOrResize);
  }
});

onBeforeUnmount(() => {
  document.removeEventListener("mousedown", onDocMouseDown, true);
  document.removeEventListener("keydown", onKeydown, true);
  window.removeEventListener("scroll", onScrollOrResize, true);
  window.removeEventListener("resize", onScrollOrResize);
});

const placementClass = computed(() => `popup-${props.placement ?? "below"}`);
const panelStyle = computed<Record<string, string>>(() => {
  const base: Record<string, string> = {};
  if (props.maxWidth) base.maxWidth = props.maxWidth;
  if (props.teleport) Object.assign(base, fixedStyle.value);
  return base;
});
</script>

<template>
  <div ref="root" class="popup-wrap">
    <slot name="trigger" :toggle="toggle" :open="open" :close="close" />

    <Teleport v-if="teleport" to="body">
      <div
        v-if="open"
        class="popup-panel popup-panel--teleported"
        :style="panelStyle"
        data-popup-panel="1"
      >
        <slot :close="close" />
      </div>
    </Teleport>

    <div
      v-else-if="open"
      class="popup-panel"
      :class="placementClass"
      :style="panelStyle"
      data-popup-panel="1"
    >
      <slot :close="close" />
    </div>
  </div>
</template>
