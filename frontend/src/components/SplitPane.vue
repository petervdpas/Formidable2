<script setup lang="ts">
import { ref, onBeforeUnmount } from "vue";

const props = withDefaults(
  defineProps<{
    initial?: number;
    min?: number;
    max?: number;
    /** When true, the sidebar splits into a fixed header region and a
     *  scrollable area marked by `.sidebar-scroll`. Use when the
     *  sidebar has filter/title controls that should stay in view. */
    sidebarSplit?: boolean;
  }>(),
  { initial: 280, min: 160, max: 600, sidebarSplit: false },
);

const width = ref(props.initial);
const dragging = ref(false);

let startX = 0;
let startWidth = 0;

function clamp(n: number) {
  return Math.max(props.min, Math.min(props.max, n));
}

function onMouseMove(e: MouseEvent) {
  width.value = clamp(startWidth + (e.clientX - startX));
}

function onMouseUp() {
  dragging.value = false;
  document.body.classList.remove("is-resizing-x");
  window.removeEventListener("mousemove", onMouseMove);
  window.removeEventListener("mouseup", onMouseUp);
}

function startDrag(e: MouseEvent) {
  dragging.value = true;
  startX = e.clientX;
  startWidth = width.value;
  document.body.classList.add("is-resizing-x");
  window.addEventListener("mousemove", onMouseMove);
  window.addEventListener("mouseup", onMouseUp);
  e.preventDefault();
}

function onKeyDown(e: KeyboardEvent) {
  const step = e.shiftKey ? 32 : 8;
  if (e.key === "ArrowLeft")  { width.value = clamp(width.value - step); e.preventDefault(); }
  if (e.key === "ArrowRight") { width.value = clamp(width.value + step); e.preventDefault(); }
}

onBeforeUnmount(onMouseUp);
</script>

<template>
  <div class="split-pane" :class="{ dragging }">
    <aside
      :class="['workspace-sidebar', { 'workspace-sidebar--split': props.sidebarSplit }]"
      :style="{ width: width + 'px' }"
    >
      <slot name="sidebar" />
    </aside>
    <div
      class="split-handle"
      role="separator"
      aria-orientation="vertical"
      tabindex="0"
      :aria-valuenow="width"
      :aria-valuemin="min"
      :aria-valuemax="max"
      @mousedown="startDrag"
      @keydown="onKeyDown"
    ></div>
    <section class="workspace-main">
      <slot name="main" />
    </section>
  </div>
</template>
