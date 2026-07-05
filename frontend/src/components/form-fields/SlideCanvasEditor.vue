<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { Service as FontsSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/fonts";
import { slideBlockComponent, SlideSettings, SlideElementTransition, SlideElementShadow, SlideElementOrder } from "./slide-blocks";
import {
  ensureSlideBlockKindsLoaded,
  slideBlockKinds,
  parseSlideDoc,
  newBlock,
  INLINE_TEXT_KINDS,
  SLIDE_CANVAS_W,
  SLIDE_CANVAS_H,
  type SlideBlock,
} from "../../types/slide-blocks";

const props = withDefaults(
  defineProps<{ modelValue: unknown; canvasW?: number; canvasH?: number }>(),
  { canvasW: SLIDE_CANVAS_W, canvasH: SLIDE_CANVAS_H },
);
const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

const templateFilename = inject<ComputedRef<string>>(
  "templateFilename",
  computed(() => ""),
);

const KIND_ICON: Record<string, string> = {
  text: "fa-paragraph", image: "fa-image", video: "fa-video",
  embed: "fa-window-maximize", code: "fa-code", math: "fa-square-root-variable",
  table: "fa-table", list: "fa-list-ul", quote: "fa-quote-right", mermaid: "fa-diagram-project",
  shape: "fa-shapes",
};
const iconFor = (kind: string) => KIND_ICON[kind] ?? "fa-square";

// ── doc state ─────────────────────────────────────────────────────────
const parsed = parseSlideDoc(props.modelValue);
const blocks = ref<SlideBlock[]>(parsed.blocks);
const background = ref(parsed.background ?? "");
const transition = ref(parsed.transition ?? "");
const notes = ref(parsed.notes ?? "");
const selectedId = ref<string | null>(null);
const editingId = ref<string | null>(null); // block being edited inline on the canvas
const inspectorOpen = ref(false); // right slideout; auto-opens on selection
watch(selectedId, (id) => { if (id) inspectorOpen.value = true; });

const blockHtml = ref<Record<string, string>>({});
const renderTimers: Record<string, ReturnType<typeof setTimeout>> = {};

// Image blocks are aspect-locked: the frame follows the image's natural ratio so
// the image fills it edge-to-edge (no letterbox). imgRatio caches naturalWidth/
// naturalHeight per block for resize; pendingSnap marks blocks whose frame should
// re-fit after the NEXT render - i.e. the user just picked/replaced an image.
// Loading a slide only probes the ratio (snap=false), so opening one never
// rewrites its frames or dirties the form.
const imgRatio = ref<Record<string, number>>({});
const pendingSnap = new Set<string>();

function probeImage(b: SlideBlock, snap: boolean) {
  const m = /<img[^>]+src="([^"]*)"/i.exec(blockHtml.value[b.id] ?? "");
  if (!m || !m[1]) return;
  const probe = new Image();
  probe.onload = () => {
    const nw = probe.naturalWidth, nh = probe.naturalHeight;
    if (!nw || !nh) return;
    imgRatio.value[b.id] = nw / nh;
    if (snap) {
      const h = Math.max(40, Math.round((b.w * nh) / nw));
      if (h !== b.h) {
        b.h = h;
        commit();
      }
    }
  };
  probe.src = m[1];
}

function renderBlock(b: SlideBlock) {
  clearTimeout(renderTimers[b.id]);
  renderTimers[b.id] = setTimeout(async () => {
    blockHtml.value[b.id] = await RenderSvc.RenderSlideBlockHTML(
      templateFilename.value, b.kind, b.content,
    );
    if (b.kind === "image") {
      const snap = pendingSnap.has(b.id);
      pendingSnap.delete(b.id);
      probeImage(b, snap);
    }
  }, 150);
}
function renderAll() {
  for (const b of blocks.value) renderBlock(b);
}
// @font-face rules for user-uploaded fonts, so a picked upload renders on the
// canvas (web-safe fonts need no face). Refetched on mount, which covers a font
// added in the Information panel before returning to the editor.
const fontFaceCss = ref("");
onMounted(() => {
  void ensureSlideBlockKindsLoaded();
  void FontsSvc.FontFaceCSS().then((css) => { fontFaceCss.value = css ?? ""; });
  renderAll();
});

watch(
  () => props.modelValue,
  (v) => {
    const p = parseSlideDoc(v);
    if (JSON.stringify(p.blocks) !== JSON.stringify(blocks.value)) {
      blocks.value = p.blocks;
      if (!blocks.value.some((b) => b.id === selectedId.value)) selectedId.value = null;
      renderAll();
    }
    background.value = p.background ?? "";
    transition.value = p.transition ?? "";
    notes.value = p.notes ?? "";
  },
);

function commit() {
  const doc: Record<string, unknown> = { blocks: JSON.parse(JSON.stringify(blocks.value)) };
  if (background.value) doc.background = background.value;
  if (transition.value) doc.transition = transition.value;
  if (notes.value) doc.notes = notes.value;
  emit("update:modelValue", doc);
}

// ── palette ───────────────────────────────────────────────────────────
const kinds = computed(() => slideBlockKinds());
const labelFor = (kind: string) => t(kinds.value.find((k) => k.name === kind)?.label_key ?? kind);

function addBlock(kind: string) {
  const b = newBlock(kind, blocks.value.length);
  blocks.value.push(b);
  selectedId.value = b.id;
  if (INLINE_TEXT_KINDS.has(kind)) editingId.value = b.id;
  renderBlock(b);
  commit();
}
function removeSelected() {
  blocks.value = blocks.value.filter((b) => b.id !== selectedId.value);
  selectedId.value = null;
  editingId.value = null;
  commit();
}
// z-order: blocks paint in array order, so swapping a block with its neighbour
// moves it one step forward (later, on top) or backward. The render (slide.go)
// and canvas both honour array order, so this is the single source of z-order.
function reorder(b: SlideBlock | null, dir: "forward" | "backward") {
  if (!b) return;
  const arr = blocks.value;
  const i = arr.indexOf(b);
  const j = dir === "forward" ? i + 1 : i - 1;
  if (i < 0 || j < 0 || j >= arr.length) return;
  const tmp = arr[i];
  arr[i] = arr[j];
  arr[j] = tmp;
  commit();
}
const selectedIndex = computed(() =>
  selected.value ? blocks.value.indexOf(selected.value) : -1,
);

const selected = computed(() => blocks.value.find((b) => b.id === selectedId.value) ?? null);
const blockComp = (kind: string) => slideBlockComponent(kind);

// Every element edit (content, lang, style) arrives as a block patch; the
// components own their own property controls, this just merges and re-renders.
function applyPatch(b: SlideBlock | null, p: Partial<SlideBlock>) {
  if (!b) return;
  Object.assign(b, p);
  if ("content" in p && b.kind === "image") pendingSnap.add(b.id);
  if ("content" in p || "lang" in p) renderBlock(b);
  commit();
}

// Inspector W/H edits keep the image's aspect: change one, the other follows.
function onGeoResize(b: SlideBlock | null, axis: "w" | "h") {
  if (!b) return;
  const ratio = b.kind === "image" ? imgRatio.value[b.id] : undefined;
  if (ratio) {
    if (axis === "w") b.h = Math.max(40, Math.round(b.w / ratio));
    else b.w = Math.max(40, Math.round(b.h * ratio));
  }
  commit();
}
function startEdit(b: SlideBlock) {
  if (!INLINE_TEXT_KINDS.has(b.kind)) return;
  selectedId.value = b.id;
  editingId.value = b.id;
}

// ── slide-level settings ───────────────────────────────────────────────
function setBackground(v: string) { background.value = v; commit(); }
function setTransition(v: string) { transition.value = v; commit(); }
function setNotes(v: string) { notes.value = v; commit(); }

// ── zoom + pan viewport ────────────────────────────────────────────────
// The stage is a fixed viewport; the canvas is translated + scaled inside it. It
// fits-to-width by default, but the user can zoom (Ctrl/Cmd + wheel, anchored at
// the cursor) and pan (Space-drag or middle-mouse-drag); "Fit" resets both. This
// keeps a large canvas (e.g. 1920x1080) workable instead of shrunk to nothing.
const stageWrap = ref<HTMLElement | null>(null);
const ZOOM_MIN = 0.1, ZOOM_MAX = 4;
const fitScale = ref(1);
const zoom = ref(1);
const pan = ref({ x: 0, y: 0 });
const userAdjusted = ref(false); // once the user zooms/pans, stop auto-fitting on resize
const spaceDown = ref(false);

const clampZoom = (z: number) => Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z));
// The viewport is a fixed working area (CSS height, independent of the canvas);
// the slide fits inside it on BOTH axes and is centred, so switching canvas
// format (16:9 <-> 4:3) never resizes the editor.
function computeFit() {
  const el = stageWrap.value;
  const w = (el?.clientWidth ?? props.canvasW) - 24;
  const h = (el?.clientHeight ?? props.canvasH) - 24;
  fitScale.value = Math.max(ZOOM_MIN, Math.min(1, w / props.canvasW, h / props.canvasH));
}
function centeredPan(z: number): { x: number; y: number } {
  const el = stageWrap.value;
  return {
    x: ((el?.clientWidth ?? 0) - props.canvasW * z) / 2,
    y: ((el?.clientHeight ?? 0) - props.canvasH * z) / 2,
  };
}
// Zoom to a target scale keeping the viewport point (vx,vy) over the same canvas
// point, so zooming stays anchored where the cursor is.
function zoomAt(target: number, vx: number, vy: number) {
  const k = target / zoom.value;
  pan.value = { x: vx - (vx - pan.value.x) * k, y: vy - (vy - pan.value.y) * k };
  zoom.value = target;
  userAdjusted.value = true;
}
function zoomBy(dir: number) {
  const rect = stageWrap.value?.getBoundingClientRect();
  zoomAt(clampZoom(zoom.value * (dir > 0 ? 1.2 : 1 / 1.2)), (rect?.width ?? 0) / 2, (rect?.height ?? 0) / 2);
}
function fitToView() {
  computeFit();
  zoom.value = fitScale.value;
  pan.value = centeredPan(fitScale.value);
  userAdjusted.value = false;
}
function onWheel(e: WheelEvent) {
  if (!e.ctrlKey && !e.metaKey) return; // otherwise let the page scroll normally
  e.preventDefault();
  const rect = stageWrap.value?.getBoundingClientRect();
  if (!rect) return;
  zoomAt(clampZoom(zoom.value * (e.deltaY < 0 ? 1.1 : 1 / 1.1)), e.clientX - rect.left, e.clientY - rect.top);
}
function startPan(e: PointerEvent) {
  if (!spaceDown.value && e.button !== 1) return; // pan only with Space or middle-mouse
  e.preventDefault();
  const sx = pan.value.x, sy = pan.value.y, px = e.clientX, py = e.clientY;
  const move = (ev: PointerEvent) => { pan.value = { x: sx + (ev.clientX - px), y: sy + (ev.clientY - py) }; };
  const up = () => {
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", up);
  };
  window.addEventListener("pointermove", move);
  window.addEventListener("pointerup", up);
  userAdjusted.value = true;
}
function isTypingTarget(t: EventTarget | null): boolean {
  const el = t as HTMLElement | null;
  return !!el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable);
}
function onKeyDown(e: KeyboardEvent) {
  if (e.code === "Space" && !isTypingTarget(e.target)) spaceDown.value = true;
}
function onKeyUp(e: KeyboardEvent) {
  if (e.code === "Space") spaceDown.value = false;
}
onMounted(() => {
  if (stageWrap.value) {
    const ro = new ResizeObserver(() => {
      computeFit();
      if (!userAdjusted.value) { zoom.value = fitScale.value; pan.value = centeredPan(fitScale.value); }
    });
    ro.observe(stageWrap.value);
    fitToView();
    // Non-passive so preventDefault holds; otherwise a Ctrl-zoom also scrolls.
    stageWrap.value.addEventListener("wheel", onWheel, { passive: false });
  }
  window.addEventListener("keydown", onKeyDown);
  window.addEventListener("keyup", onKeyUp);
});
onBeforeUnmount(() => {
  stageWrap.value?.removeEventListener("wheel", onWheel);
  window.removeEventListener("keydown", onKeyDown);
  window.removeEventListener("keyup", onKeyUp);
});
// Refit (and recentre) when the canvas format changes, so a new aspect ratio
// lands centred in the same fixed viewport instead of clipped or off-corner.
watch(() => [props.canvasW, props.canvasH], () => fitToView());
const stageStyle = computed(() => ({
  width: `${props.canvasW}px`,
  height: `${props.canvasH}px`,
  transform: `translate(${pan.value.x}px, ${pan.value.y}px) scale(${zoom.value})`,
  transformOrigin: "top left",
  background: background.value || "#fff",
}));
// The box is exactly the stored geometry; content sits inside it. z-index is the
// array index (matching render.renderSlide), so editor and deck stack identically.
function blockStyle(b: SlideBlock, i: number) {
  return { left: `${b.x}px`, top: `${b.y}px`, width: `${b.w}px`, height: `${b.h}px`, zIndex: i };
}

// ── grid + smart guides ───────────────────────────────────────────────
// PowerPoint-style: free placement, but the editor is aware of the other blocks
// and the canvas. While dragging/resizing, an edge or centre that lines up with
// another block (or the canvas edge/centre) snaps to it and a guide line shows
// the match. A grid toggle adds plain grid snapping as a fallback.
const GRID = 20; // canvas px
const SNAP = 6; // snap threshold in canvas px
const showGrid = ref(false);
const guides = ref<{ x: number[]; y: number[] }>({ x: [], y: [] });

// Candidate snap lines: the canvas edges + centre, plus every OTHER block's
// near / centre / far edge on that axis.
function targetsX(selfId: string): number[] {
  const t = [0, props.canvasW / 2, props.canvasW];
  for (const o of blocks.value) if (o.id !== selfId) t.push(o.x, o.x + o.w / 2, o.x + o.w);
  return t;
}
function targetsY(selfId: string): number[] {
  const t = [0, props.canvasH / 2, props.canvasH];
  for (const o of blocks.value) if (o.id !== selfId) t.push(o.y, o.y + o.h / 2, o.y + o.h);
  return t;
}
// Snap a box (its near/centre/far edges) to the nearest target; returns the
// snapped leading-edge value and the guide line, or null guide if nothing hit.
function snapBox(lead: number, size: number, targets: number[]): { value: number; guide: number | null } {
  const edges = [lead, lead + size / 2, lead + size];
  const offset = [0, size / 2, size];
  let bestD = SNAP, value = lead, guide: number | null = null;
  for (const t of targets)
    for (let i = 0; i < 3; i++) {
      const d = Math.abs(edges[i] - t);
      if (d < bestD) { bestD = d; value = t - offset[i]; guide = t; }
    }
  return { value, guide };
}
// Snap a single moving point (a resize edge) to the nearest target.
function snapPoint(v: number, targets: number[]): { value: number; guide: number | null } {
  let bestD = SNAP, value = v, guide: number | null = null;
  for (const t of targets) {
    const d = Math.abs(v - t);
    if (d < bestD) { bestD = d; value = t; guide = t; }
  }
  return { value, guide };
}
function toGrid(v: number): number {
  return showGrid.value ? Math.round(v / GRID) * GRID : Math.round(v);
}
function clearGuides() {
  guides.value = { x: [], y: [] };
}

// ── drag / resize ─────────────────────────────────────────────────────
function startDrag(b: SlideBlock, e: PointerEvent) {
  if (e.button !== 0 || editingId.value === b.id || spaceDown.value) return; // Space-drag pans instead
  selectedId.value = b.id;
  const sx = b.x, sy = b.y, px = e.clientX, py = e.clientY;
  const el = e.currentTarget as HTMLElement;
  el.setPointerCapture(e.pointerId);
  const move = (ev: PointerEvent) => {
    const rawX = sx + (ev.clientX - px) / zoom.value;
    const rawY = sy + (ev.clientY - py) / zoom.value;
    const snX = snapBox(rawX, b.w, targetsX(b.id));
    const snY = snapBox(rawY, b.h, targetsY(b.id));
    b.x = Math.max(0, snX.guide !== null ? Math.round(snX.value) : toGrid(rawX));
    b.y = Math.max(0, snY.guide !== null ? Math.round(snY.value) : toGrid(rawY));
    guides.value = { x: snX.guide !== null ? [snX.guide] : [], y: snY.guide !== null ? [snY.guide] : [] };
  };
  const up = (ev: PointerEvent) => {
    el.releasePointerCapture(ev.pointerId);
    el.removeEventListener("pointermove", move);
    el.removeEventListener("pointerup", up);
    clearGuides();
    commit();
  };
  el.addEventListener("pointermove", move);
  el.addEventListener("pointerup", up);
}
function startResize(b: SlideBlock, e: PointerEvent) {
  if (e.button !== 0) return;
  e.stopPropagation();
  const sw = b.w, sh = b.h, px = e.clientX, py = e.clientY;
  const el = e.currentTarget as HTMLElement;
  el.setPointerCapture(e.pointerId);
  // Image frames are aspect-locked to the image: width leads, height follows.
  const ratio = b.kind === "image" ? imgRatio.value[b.id] : undefined;
  const move = (ev: PointerEvent) => {
    const rawRight = b.x + Math.max(40, sw + (ev.clientX - px) / zoom.value);
    const snR = snapPoint(rawRight, targetsX(b.id));
    const right = snR.guide !== null ? Math.round(snR.value) : toGrid(rawRight);
    const nw = Math.max(40, right - b.x);
    if (ratio) {
      b.w = nw;
      b.h = Math.max(40, Math.round(nw / ratio));
      guides.value = { x: snR.guide !== null ? [snR.guide] : [], y: [] };
    } else {
      const rawBottom = b.y + Math.max(40, sh + (ev.clientY - py) / zoom.value);
      const snB = snapPoint(rawBottom, targetsY(b.id));
      const bottom = snB.guide !== null ? Math.round(snB.value) : toGrid(rawBottom);
      b.w = nw;
      b.h = Math.max(40, bottom - b.y);
      guides.value = { x: snR.guide !== null ? [snR.guide] : [], y: snB.guide !== null ? [snB.guide] : [] };
    }
  };
  const up = (ev: PointerEvent) => {
    el.releasePointerCapture(ev.pointerId);
    el.removeEventListener("pointermove", move);
    el.removeEventListener("pointerup", up);
    clearGuides();
    commit();
  };
  el.addEventListener("pointermove", move);
  el.addEventListener("pointerup", up);
}
</script>

<template>
  <div class="slide-editor">
    <component v-if="fontFaceCss" :is="'style'" v-text="fontFaceCss" />
    <div class="slide-editor-body">
      <div class="slide-toolrail">
        <button
          type="button" class="slide-tool slide-tool-toggle"
          :class="{ active: showGrid }" :aria-pressed="showGrid"
          :title="t('workspace.storage.slide.snap_grid')"
          @click="showGrid = !showGrid"
        >
          <i class="fa-solid fa-border-all" aria-hidden="true"></i>
          <span>{{ t('workspace.storage.slide.snap_grid') }}</span>
        </button>
        <span class="slide-toolrail-label">{{ t('workspace.storage.slide.add') }}</span>
        <button
          v-for="k in kinds" :key="k.name" type="button" class="slide-tool"
          :title="t(k.label_key)" @click="addBlock(k.name)"
        >
          <i class="fa-solid" :class="iconFor(k.name)" aria-hidden="true"></i>
          <span>{{ t(k.label_key) }}</span>
        </button>
      </div>

      <div
        ref="stageWrap" class="slide-stage-wrap" :class="{ panning: spaceDown }"
        @pointerdown="startPan"
      >
        <div class="slide-stage" :style="stageStyle" @pointerdown.self="selectedId = null; editingId = null">
            <div
              v-if="showGrid" class="slide-grid"
              :style="{ backgroundSize: `${GRID}px ${GRID}px` }"
            ></div>
            <div
              v-for="(b, i) in blocks" :key="b.id"
              class="slide-block-box"
              :class="[`slide-block-${b.kind}`, b.shadow ? `slide-shadow-${b.shadow}` : '', b.shadow && b.shadowDir ? `slide-shadow-dir-${b.shadowDir}` : '', { selected: b.id === selectedId, editing: b.id === editingId }]"
              :style="blockStyle(b, i)"
              @pointerdown="startDrag(b, $event)"
              @dblclick="startEdit(b)"
            >
              <component
                :is="blockComp(b.kind)"
                surface="canvas"
                :block="b"
                :html="blockHtml[b.id] ?? ''"
                :editing="b.id === editingId"
                @patch="applyPatch(b, $event)"
                @end-edit="editingId = null"
              />
              <span v-if="b.fragment" class="slide-block-box-frag">{{ b.fragment }}</span>
              <span v-if="b.id === selectedId" class="slide-block-box-dim">{{ b.w }} × {{ b.h }}</span>
              <div class="slide-block-box-resize" @pointerdown="startResize(b, $event)"></div>
            </div>
            <div
              v-for="gx in guides.x" :key="`gx${gx}`"
              class="slide-guide slide-guide-v" :style="{ left: `${gx}px` }"
            ></div>
            <div
              v-for="gy in guides.y" :key="`gy${gy}`"
              class="slide-guide slide-guide-h" :style="{ top: `${gy}px` }"
            ></div>
        </div>
        <div class="slide-zoombar">
          <button type="button" class="slide-zoom-btn" :title="t('workspace.storage.slide.zoom_out')" @click="zoomBy(-1)">
            <i class="fa-solid fa-minus" aria-hidden="true"></i>
          </button>
          <button type="button" class="slide-zoom-btn slide-zoom-fit" :title="t('workspace.storage.slide.zoom_fit')" @click="fitToView">
            {{ Math.round(zoom * 100) }}%
          </button>
          <button type="button" class="slide-zoom-btn" :title="t('workspace.storage.slide.zoom_in')" @click="zoomBy(1)">
            <i class="fa-solid fa-plus" aria-hidden="true"></i>
          </button>
        </div>
      </div>

      <button
        type="button" class="slide-inspector-rail"
        :title="t('workspace.storage.slide.properties')"
        @click="inspectorOpen = !inspectorOpen"
      >
        <i class="fa-solid" :class="inspectorOpen ? 'fa-chevron-right' : 'fa-chevron-left'" aria-hidden="true"></i>
      </button>
      <aside v-show="inspectorOpen" class="slide-inspector">
        <template v-if="selected">
          <div class="slide-inspector-head">
            <i class="fa-solid" :class="iconFor(selected.kind)" aria-hidden="true"></i>
            <span>{{ labelFor(selected.kind) }}</span>
          </div>
          <div class="slide-inspector-geo">
            <label>X<input type="number" v-model.number="selected.x" @change="commit" /></label>
            <label>Y<input type="number" v-model.number="selected.y" @change="commit" /></label>
            <label>W<input type="number" v-model.number="selected.w" @change="onGeoResize(selected, 'w')" /></label>
            <label>H<input type="number" v-model.number="selected.h" @change="onGeoResize(selected, 'h')" /></label>
          </div>

          <!-- content + type-specific properties: each element owns its editor -->
          <div class="slide-inspector-content">
            <component
              :is="blockComp(selected.kind)"
              surface="inspector"
              :block="selected"
              @patch="applyPatch(selected, $event)"
            />
          </div>

          <SlideElementShadow :block="selected" @patch="applyPatch(selected, $event)" />

          <SlideElementTransition :block="selected" @patch="applyPatch(selected, $event)" />

          <SlideElementOrder
            :can-forward="selectedIndex >= 0 && selectedIndex < blocks.length - 1"
            :can-backward="selectedIndex > 0"
            @forward="reorder(selected, 'forward')"
            @backward="reorder(selected, 'backward')"
          />

          <button type="button" class="tool-btn danger" @click="removeSelected">
            {{ t('workspace.storage.slide.delete_block') }}
          </button>
        </template>

        <!-- No element selected: the panel edits the slide itself. -->
        <template v-else>
          <p class="muted small">{{ t('workspace.storage.slide.no_selection') }}</p>
          <hr class="slide-inspector-sep" />
          <SlideSettings
            :background="background" :transition="transition" :notes="notes"
            @update:background="setBackground" @update:transition="setTransition" @update:notes="setNotes"
          />
        </template>
      </aside>
    </div>
  </div>
</template>
