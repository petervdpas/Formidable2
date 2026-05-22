<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useViewportWidth } from "../composables/useViewportWidth";

const { t } = useI18n();

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

const { narrow } = useViewportWidth();

// Overlay open/closed state - only meaningful while `narrow` is true.
// Auto-closes whenever the viewport widens past the breakpoint so the
// in-flow sidebar reappears in its normal position.
const overlayOpen = ref(false);
watch(narrow, (isNarrow) => {
  if (!isNarrow) overlayOpen.value = false;
});

function toggleOverlay() {
  overlayOpen.value = !overlayOpen.value;
}
function closeOverlay() {
  overlayOpen.value = false;
}

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

// Sidebar inline style: fixed resizable width in normal mode; CSS
// owns the width in narrow/overlay mode (via .workspace-sidebar--overlay).
const sidebarStyle = computed(() => (narrow.value ? {} : { width: width.value + "px" }));

const toggleGlyph = computed(() => (overlayOpen.value ? "«" : "»"));
</script>

<template>
  <div
    :class="[
      'split-pane',
      {
        dragging,
        'split-pane--narrow': narrow,
        'split-pane--overlay-open': narrow && overlayOpen,
      },
    ]"
  >
    <aside
      :class="[
        'workspace-sidebar',
        {
          'workspace-sidebar--split': props.sidebarSplit,
          'workspace-sidebar--overlay': narrow,
        },
      ]"
      :style="sidebarStyle"
    >
      <slot name="sidebar" />
    </aside>
    <button
      v-if="narrow"
      type="button"
      class="split-pane-edge-toggle"
      :aria-label="overlayOpen ? t('common.sidebar_close') : t('common.sidebar_open')"
      :aria-expanded="overlayOpen"
      @click="toggleOverlay"
    >{{ toggleGlyph }}</button>
    <div
      v-if="!narrow"
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

    <div
      v-if="narrow && overlayOpen"
      class="split-pane-backdrop"
      @click="closeOverlay"
    ></div>
  </div>
</template>
