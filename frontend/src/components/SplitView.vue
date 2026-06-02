<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";

// Generic horizontal two-pane split with a draggable divider. Fills its
// parent (give the parent a height). Sizes are fractions of the width;
// the first pane gets `ratio`, the second takes the rest. Reuses the
// app-wide `.split-handle` bar look and the `is-resizing-x` body cursor.
const props = withDefaults(
  defineProps<{
    /** First pane's initial fraction of the width (0..1). */
    initial?: number;
    /** Minimum fraction for either pane. */
    min?: number;
  }>(),
  { initial: 0.5, min: 0.15 },
);

function clampRatio(r: number): number {
  return Math.max(props.min, Math.min(1 - props.min, r));
}

const ratio = ref(clampRatio(props.initial));
const rootRef = ref<HTMLElement | null>(null);
const dragging = ref(false);

function onMove(e: MouseEvent) {
  const el = rootRef.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  if (rect.width <= 0) return;
  ratio.value = clampRatio((e.clientX - rect.left) / rect.width);
}

function onUp() {
  dragging.value = false;
  document.body.classList.remove("is-resizing-x");
  window.removeEventListener("mousemove", onMove);
  window.removeEventListener("mouseup", onUp);
}

function startDrag(e: MouseEvent) {
  dragging.value = true;
  document.body.classList.add("is-resizing-x");
  window.addEventListener("mousemove", onMove);
  window.addEventListener("mouseup", onUp);
  e.preventDefault();
}

function onKey(e: KeyboardEvent) {
  const step = e.shiftKey ? 0.05 : 0.02;
  if (e.key === "ArrowLeft") {
    ratio.value = clampRatio(ratio.value - step);
    e.preventDefault();
  }
  if (e.key === "ArrowRight") {
    ratio.value = clampRatio(ratio.value + step);
    e.preventDefault();
  }
}

onBeforeUnmount(onUp);

const firstStyle = computed(() => ({
  flexBasis: `${(ratio.value * 100).toFixed(3)}%`,
}));
const pct = computed(() => Math.round(ratio.value * 100));
</script>

<template>
  <div ref="rootRef" class="split-view" :class="{ dragging }">
    <div class="split-view-pane split-view-pane--first" :style="firstStyle">
      <slot name="first" />
    </div>
    <div
      class="split-handle"
      role="separator"
      aria-orientation="vertical"
      tabindex="0"
      :aria-valuenow="pct"
      :aria-valuemin="Math.round(min * 100)"
      :aria-valuemax="Math.round((1 - min) * 100)"
      @mousedown="startDrag"
      @keydown="onKey"
    ></div>
    <div class="split-view-pane split-view-pane--second">
      <slot name="second" />
    </div>
  </div>
</template>
