<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";

// ImageLightbox — fullscreen image viewer with pan + zoom.
// Mirrors the original Formidable's image modal:
//   - Ctrl+Wheel  → zoom
//   - + / - keys  → zoom
//   - Esc         → close
//   - Click outside the image → close
//   - Drag inside the image → pan when zoomed in

const props = defineProps<{
  open: boolean;
  src: string;
  alt?: string;
}>();

const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const MIN_SCALE = 0.2;
const MAX_SCALE = 8;
const STEP = 0.15;

const scale = ref(1);
const tx = ref(0);
const ty = ref(0);
const dragging = ref(false);
let dragStart: { x: number; y: number; tx: number; ty: number } | null = null;

// Compose: centre via -50%/-50% (scale & pan pivot on the image's
// centre), then user pan, then scale.
const transform = computed(
  () =>
    `translate(-50%, -50%) translate(${tx.value}px, ${ty.value}px) scale(${scale.value})`,
);

function reset() {
  scale.value = 1;
  tx.value = 0;
  ty.value = 0;
}

function close() {
  emit("close");
}

function onBackdropClick(e: MouseEvent) {
  // Only close when clicking the backdrop itself, not the image.
  if ((e.target as HTMLElement).dataset.role === "backdrop") close();
}

function zoomBy(delta: number) {
  const next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale.value + delta));
  scale.value = next;
}

function onWheel(e: WheelEvent) {
  if (!e.ctrlKey && !e.metaKey) return;
  e.preventDefault();
  zoomBy(e.deltaY < 0 ? STEP : -STEP);
}

function onKey(e: KeyboardEvent) {
  if (!props.open) return;
  if (e.key === "Escape") {
    e.preventDefault();
    close();
    return;
  }
  if (e.key === "+" || e.key === "=") {
    e.preventDefault();
    zoomBy(STEP);
    return;
  }
  if (e.key === "-" || e.key === "_") {
    e.preventDefault();
    zoomBy(-STEP);
    return;
  }
  if (e.key === "0") {
    e.preventDefault();
    reset();
  }
}

function onPointerDown(e: PointerEvent) {
  dragging.value = true;
  dragStart = { x: e.clientX, y: e.clientY, tx: tx.value, ty: ty.value };
  (e.target as Element).setPointerCapture?.(e.pointerId);
}

function onPointerMove(e: PointerEvent) {
  if (!dragging.value || !dragStart) return;
  tx.value = dragStart.tx + (e.clientX - dragStart.x);
  ty.value = dragStart.ty + (e.clientY - dragStart.y);
}

function onPointerUp() {
  dragging.value = false;
  dragStart = null;
}

// Window-scoped key handler — only attached while the lightbox is
// open so Esc/+/- don't fire when it's closed.
watch(
  () => props.open,
  (open) => {
    if (open) {
      reset();
      window.addEventListener("keydown", onKey);
    } else {
      window.removeEventListener("keydown", onKey);
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKey);
});
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="image-lightbox-backdrop"
      data-role="backdrop"
      @click="onBackdropClick"
      @wheel="onWheel"
    >
      <div class="image-lightbox-help">
        <div>
          <kbd>Ctrl</kbd> + <kbd>{{ t('image_lightbox.scroll') }}</kbd>
          {{ t('image_lightbox.to_zoom') }}
        </div>
        <div>
          <kbd>+</kbd> / <kbd>−</kbd>
          {{ t('image_lightbox.or') }}
          <kbd>Esc</kbd>
          {{ t('image_lightbox.to_close') }}
        </div>
      </div>

      <img
        :src="src"
        :alt="alt ?? ''"
        class="image-lightbox-img"
        :class="{ 'is-grabbing': dragging }"
        :style="{ transform }"
        draggable="false"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
      />
    </div>
  </Teleport>
</template>
