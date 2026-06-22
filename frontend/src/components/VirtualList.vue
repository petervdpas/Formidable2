<script setup lang="ts" generic="T">
// Windowed list that renders only the rows intersecting the viewport (plus
// an overscan margin), so a collection of thousands of records mounts a few
// dozen DOM nodes rather than all of them.
//
// Rows may have DIFFERENT heights (a sidebar row with a sub-label is taller
// than one without). Each row's real height is measured from the live DOM as
// it scrolls into view and cached by index; positions come from a prefix-sum
// over those heights, with an average-of-known estimate standing in for rows
// not yet measured. That keeps the reserved height honest (the last row is
// always reachable) and lets scrollToKey land on a row's true position rather
// than a synthetic stride that drifts when rows aren't uniform.
//
// Measurement happens in onUpdated and scroll corrections in rAF, never in a
// watcher on the render output: heights/scrollTop feed back into the rendered
// window, and mutating them inside the render flush corrupts Vue's block patch.
//
// The component IS the scroll viewport. Consumers render each row via the
// default slot ({ item, index }) and drive centering through scrollToKey(),
// which works by index and so does not require the target row to be mounted.

import { computed, onBeforeUnmount, onMounted, onUpdated, ref, watch } from "vue";

const props = defineProps<{
  items: readonly T[];
  /** Stable per-item key; also how scrollToKey locates a row. */
  itemKey: (item: T) => string;
  /** Rows rendered above and below the viewport, each side. */
  overscan?: number;
  /** Row height used before any row has been measured. */
  estimateHeight?: number;
  /** Key of the row to center once the list is first ready (e.g. after a
   *  remount). Centering happens once per mount, so later changes to it
   *  (clicks, arrow-nav) don't yank the viewport. */
  activeKey?: string;
}>();

const overscan = computed(() => props.overscan ?? 8);

const viewport = ref<HTMLElement | null>(null);
const windowEl = ref<HTMLElement | null>(null);

const scrollTop = ref(0);
const viewportH = ref(0);

// Per-index measured row height (0 = not measured yet). Reset when the item
// set changes (a template switch reloads entirely).
const heights = ref<number[]>([]);

const avgKnown = computed(() => {
  let sum = 0;
  let count = 0;
  for (const h of heights.value) {
    if (h > 0) {
      sum += h;
      count += 1;
    }
  }
  return count ? sum / count : 0;
});
const estimate = computed(() => avgKnown.value || props.estimateHeight || 44);

// Prefix sums of row heights: offsets[i] is the top of row i, offsets[n] the
// total content height. Unmeasured rows contribute the estimate.
const offsets = computed(() => {
  const n = props.items.length;
  const est = estimate.value;
  const arr = new Array<number>(n + 1);
  arr[0] = 0;
  for (let i = 0; i < n; i++) arr[i + 1] = arr[i] + (heights.value[i] || est);
  return arr;
});
// Ceil so a fractional sum (device-pixel snapping on fractional-DPR displays,
// e.g. Windows at 125%/150%) never reserves less than the real content and
// clips the last row.
const total = computed(() => Math.ceil(offsets.value[props.items.length] || 0));

// Largest i with offsets[i] <= y.
function indexAt(y: number): number {
  const arr = offsets.value;
  let lo = 0;
  let hi = arr.length - 1;
  let ans = 0;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (arr[mid] <= y) {
      ans = mid;
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }
  return ans;
}

const startIndex = computed(() =>
  Math.max(0, indexAt(scrollTop.value) - overscan.value),
);
const endIndex = computed(() =>
  Math.min(
    props.items.length,
    indexAt(scrollTop.value + viewportH.value) + overscan.value + 1,
  ),
);
const offsetY = computed(() => offsets.value[startIndex.value] || 0);

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

function readViewport() {
  const el = viewport.value;
  if (el) viewportH.value = el.clientHeight;
}

// Record the real height (top-to-top stride, so per-row margin is included)
// of every currently rendered row. Only writes when a height actually changed
// by more than half a pixel, so a settled list stops re-rendering. Runs in
// onUpdated, so the writes schedule a fresh render rather than re-entering the
// current one. Returns whether anything changed.
function measure(): boolean {
  const win = windowEl.value;
  if (!win) return false;
  const kids = win.children;
  const n = kids.length;
  if (!n) return false;
  const rects: DOMRect[] = [];
  for (let k = 0; k < n; k++) {
    rects.push((kids[k] as HTMLElement).getBoundingClientRect());
  }
  const gap = n >= 2 ? Math.max(0, rects[1].top - rects[0].bottom) : 0;
  let changed = false;
  for (let k = 0; k < n; k++) {
    const abs = startIndex.value + k;
    const h = k < n - 1 ? rects[k + 1].top - rects[k].top : rects[k].height + gap;
    if (h > 0 && Math.abs((heights.value[abs] || 0) - h) > 0.5) {
      heights.value[abs] = h;
      changed = true;
    }
  }
  return changed;
}

// A scroll target requested before the list is ready (or before the target
// row's true position is known) is held here and re-applied as measurements
// sharpen, until the row settles within a pixel of its goal.
let pendingKey: string | null = null;
let pendingCenter = false;
let rafId = 0;

function scheduleRefine() {
  if (pendingKey === null || rafId) return;
  rafId = requestAnimationFrame(() => {
    rafId = 0;
    refine();
  });
}

function indexOfKey(key: string): number {
  return props.items.findIndex((it) => props.itemKey(it) === key);
}

// Applied outside the render flush (rAF / scroll event), so mutating scrollTop
// here is safe. Each pass nudges the target row toward its goal; the resulting
// render re-measures and schedules the next pass until it settles.
function refine() {
  const el = viewport.value;
  if (!el || pendingKey === null) return;
  if (viewportH.value <= 0 || props.items.length === 0) return;
  const index = indexOfKey(pendingKey);
  if (index < 0) {
    pendingKey = null;
    return;
  }
  const maxScroll = Math.max(0, el.scrollHeight - el.clientHeight);
  const child = windowEl.value?.children[index - startIndex.value] as
    | HTMLElement
    | undefined;

  if (child) {
    // On screen: correct by its true pixel position, independent of the
    // estimated offsets of rows above it.
    const vp = el.getBoundingClientRect();
    const rr = child.getBoundingClientRect();
    let delta: number;
    if (pendingCenter) {
      delta = rr.top - vp.top - (viewportH.value - rr.height) / 2;
    } else if (rr.top < vp.top) {
      delta = rr.top - vp.top;
    } else if (rr.bottom > vp.bottom) {
      delta = rr.bottom - vp.bottom;
    } else {
      delta = 0;
    }
    const want = Math.max(0, Math.min(el.scrollTop + delta, maxScroll));
    if (Math.abs(delta) <= 1 || Math.abs(want - el.scrollTop) <= 1) {
      el.scrollTop = want;
      scrollTop.value = want;
      pendingKey = null; // centered, or as close as the content edge allows
      return;
    }
    el.scrollTop = want;
    scrollTop.value = want;
    return;
  }

  // Not mounted yet: jump roughly via the offset model so it enters the
  // window, then the on-screen branch takes over on the next render.
  const o = offsets.value;
  const h = heights.value[index] || estimate.value;
  const top = o[index] ?? index * estimate.value;
  const want = Math.max(
    0,
    Math.min(pendingCenter ? top - (viewportH.value - h) / 2 : top, maxScroll),
  );
  if (Math.abs(el.scrollTop - want) > 1) {
    el.scrollTop = want;
    scrollTop.value = want;
  }
}

function scrollToKey(key: string, opts?: { center?: boolean }): boolean {
  if (!key || indexOfKey(key) < 0) return false;
  pendingKey = key;
  pendingCenter = !!opts?.center;
  scheduleRefine();
  return viewportH.value > 0;
}

let ro: ResizeObserver | null = null;
onMounted(() => {
  readViewport();
  ro = new ResizeObserver(() => {
    readViewport();
    scheduleRefine();
  });
  if (viewport.value) ro.observe(viewport.value);
});
onBeforeUnmount(() => {
  ro?.disconnect();
  ro = null;
  if (rafId) cancelAnimationFrame(rafId);
});

// Measure after every render and keep a pending scroll converging. Lifecycle
// hook (post-patch), so height writes schedule a new render safely.
onUpdated(() => {
  measure();
  if (pendingKey !== null) scheduleRefine();
});

// A fresh item set (template switch, filter) invalidates the cached heights.
watch(
  () => props.items,
  () => {
    heights.value = [];
    const el = viewport.value;
    if (el) scrollTop.value = el.scrollTop;
  },
);

// Center the active row once the list is ready. Locked after the first time
// per mount so clicks and arrow-nav don't pull the viewport around; a fresh
// mount starts unlocked and re-centers (the remount-restore case).
let didInitial = false;
watch(
  () => [viewportH.value, props.items.length, props.activeKey] as const,
  () => {
    if (didInitial || !props.activeKey) return;
    if (viewportH.value <= 0 || props.items.length === 0) return;
    if (indexOfKey(props.activeKey) < 0) return;
    didInitial = true;
    scrollToKey(props.activeKey, { center: true });
  },
  { immediate: true },
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
