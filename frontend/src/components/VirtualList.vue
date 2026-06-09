<script setup lang="ts" generic="T">
// Fixed-stride windowed list. Renders only the rows intersecting the
// viewport (plus an overscan margin) so a collection of thousands of
// records mounts a few dozen DOM nodes, not all of them. Rows are
// assumed uniform height; the stride is measured from the live DOM
// (first two rendered rows, so margins/borders are included) rather
// than hardcoded, then windowing math is pure arithmetic on the index.
//
// The component IS the scroll viewport. Consumers render each row via
// the default slot ({ item, index }) and drive centering through the
// exposed scrollToKey(), which works by index and so does not require
// the target row to be currently mounted.

import { computed, onBeforeUnmount, onMounted, ref, watch, nextTick } from "vue";

const props = defineProps<{
  items: readonly T[];
  /** Stable per-item key; also how scrollToKey locates a row. */
  itemKey: (item: T) => string;
  /** Rows rendered above and below the viewport, each side. */
  overscan?: number;
  /** Stride used before the first measurement settles. */
  estimateHeight?: number;
}>();

const overscan = computed(() => props.overscan ?? 8);

const viewport = ref<HTMLElement | null>(null);
const windowEl = ref<HTMLElement | null>(null);

const scrollTop = ref(0);
const viewportH = ref(0);
const measured = ref(0);

const rowH = computed(() => measured.value || props.estimateHeight || 44);
const total = computed(() => props.items.length * rowH.value);

const startIndex = computed(() => {
  const raw = Math.floor(scrollTop.value / rowH.value) - overscan.value;
  return Math.max(0, raw);
});
const endIndex = computed(() => {
  const visible = Math.ceil(viewportH.value / rowH.value) + overscan.value * 2;
  return Math.min(props.items.length, startIndex.value + visible);
});
const offsetY = computed(() => startIndex.value * rowH.value);

const rendered = computed(() =>
  props.items.slice(startIndex.value, endIndex.value).map((item, i) => ({
    item,
    index: startIndex.value + i,
    key: props.itemKey(item),
  })),
);

function onScroll() {
  const el = viewport.value;
  if (el) scrollTop.value = el.scrollTop;
}

// Measure the row stride from two adjacent rendered rows so per-row
// margin/gap is captured; fall back to a single row's box when only one
// is rendered. A short or unsettled viewport (clientHeight ~0 before
// flex layout assigns it) leaves `measured` at 0 and the estimate stands.
function measure() {
  const win = windowEl.value;
  if (!win) return;
  const kids = win.children;
  if (kids.length >= 2) {
    const a = (kids[0] as HTMLElement).getBoundingClientRect();
    const b = (kids[1] as HTMLElement).getBoundingClientRect();
    const stride = b.top - a.top;
    if (stride > 0) measured.value = stride;
  } else if (kids.length === 1) {
    const h = (kids[0] as HTMLElement).getBoundingClientRect().height;
    if (h > 0 && !measured.value) measured.value = h;
  }
}

function readViewport() {
  const el = viewport.value;
  if (el) viewportH.value = el.clientHeight;
}

// A scroll target requested before the viewport has a real height (splash
// still blocking layout) is held here and applied once a resize gives the
// viewport a usable size.
let pendingKey: string | null = null;
let pendingCenter = false;

function applyScroll(index: number, center: boolean): boolean {
  const el = viewport.value;
  if (!el || viewportH.value <= 0) return false;
  const h = rowH.value;
  const rowTop = index * h;
  const rowBottom = rowTop + h;
  const viewTop = scrollTop.value;
  const viewBottom = viewTop + viewportH.value;
  if (rowTop >= viewTop && rowBottom <= viewBottom) return true; // already visible

  let target: number;
  if (center) {
    target = rowTop - (viewportH.value - h) / 2;
  } else if (rowTop < viewTop) {
    target = rowTop;
  } else {
    target = rowBottom - viewportH.value;
  }
  const maxScroll = Math.max(0, total.value - viewportH.value);
  el.scrollTop = Math.max(0, Math.min(target, maxScroll));
  return true;
}

function scrollToKey(key: string, opts?: { center?: boolean }): boolean {
  if (!key) return false;
  const index = props.items.findIndex((it) => props.itemKey(it) === key);
  if (index < 0) return false;
  const center = !!opts?.center;
  if (applyScroll(index, center)) {
    pendingKey = null;
    return true;
  }
  pendingKey = key;
  pendingCenter = center;
  return false;
}

let ro: ResizeObserver | null = null;
onMounted(() => {
  readViewport();
  void nextTick(measure);
  ro = new ResizeObserver(() => {
    readViewport();
    measure();
    if (pendingKey) scrollToKey(pendingKey, { center: pendingCenter });
  });
  if (viewport.value) ro.observe(viewport.value);
});
onBeforeUnmount(() => {
  ro?.disconnect();
  ro = null;
});

// When the item set changes (template switch, filter), the stride may
// differ and the browser can silently clamp scrollTop against the new,
// shorter content without firing a scroll event - which would leave our
// tracked scrollTop stale and the window blank. Re-measure and resync
// from the live element after the DOM settles.
watch(
  () => props.items,
  () => {
    void nextTick(() => {
      measure();
      const el = viewport.value;
      if (el) scrollTop.value = el.scrollTop;
    });
  },
);

defineExpose({ scrollToKey, el: viewport });
</script>

<template>
  <div ref="viewport" class="virtual-list" @scroll="onScroll">
    <div class="virtual-list-spacer" :style="{ height: total + 'px' }">
      <ul
        ref="windowEl"
        class="virtual-list-window"
        :style="{ transform: `translateY(${offsetY}px)` }"
      >
        <slot
          v-for="row in rendered"
          :key="row.key"
          :item="row.item"
          :index="row.index"
        />
      </ul>
    </div>
  </div>
</template>
