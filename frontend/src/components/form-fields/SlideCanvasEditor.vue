<script setup lang="ts">
import { computed, inject, onMounted, ref, watch, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { slideBlockComponent, SlideSettings, SlideElementTransition } from "./slide-blocks";
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

function renderBlock(b: SlideBlock) {
  clearTimeout(renderTimers[b.id]);
  renderTimers[b.id] = setTimeout(async () => {
    blockHtml.value[b.id] = await RenderSvc.RenderSlideBlockHTML(
      templateFilename.value, b.kind, b.content,
    );
  }, 150);
}
function renderAll() {
  for (const b of blocks.value) renderBlock(b);
}
onMounted(() => {
  void ensureSlideBlockKindsLoaded();
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

const selected = computed(() => blocks.value.find((b) => b.id === selectedId.value) ?? null);
const blockComp = (kind: string) => slideBlockComponent(kind);

// Every element edit (content, lang, style) arrives as a block patch; the
// components own their own property controls, this just merges and re-renders.
function applyPatch(b: SlideBlock | null, p: Partial<SlideBlock>) {
  if (!b) return;
  Object.assign(b, p);
  if ("content" in p || "lang" in p) renderBlock(b);
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

// ── stage scaling ─────────────────────────────────────────────────────
const stageWrap = ref<HTMLElement | null>(null);
const scale = ref(1);
onMounted(() => {
  if (!stageWrap.value) return;
  const ro = new ResizeObserver(() => {
    const w = (stageWrap.value?.clientWidth ?? props.canvasW) - 24;
    scale.value = Math.min(1, w / props.canvasW);
  });
  ro.observe(stageWrap.value);
});
const stageStyle = computed(() => ({
  width: `${props.canvasW}px`,
  height: `${props.canvasH}px`,
  transform: `scale(${scale.value})`,
  transformOrigin: "top left",
  background: background.value || "#fff",
}));
const wrapSpacerStyle = computed(() => ({
  width: `${props.canvasW * scale.value}px`,
  height: `${props.canvasH * scale.value}px`,
}));
function blockStyle(b: SlideBlock) {
  return { left: `${b.x}px`, top: `${b.y}px`, width: `${b.w}px`, height: `${b.h}px` };
}

// ── drag / resize ─────────────────────────────────────────────────────
function startDrag(b: SlideBlock, e: PointerEvent) {
  if (e.button !== 0 || editingId.value === b.id) return;
  selectedId.value = b.id;
  const sx = b.x, sy = b.y, px = e.clientX, py = e.clientY;
  const el = e.currentTarget as HTMLElement;
  el.setPointerCapture(e.pointerId);
  const move = (ev: PointerEvent) => {
    b.x = Math.max(0, Math.round(sx + (ev.clientX - px) / scale.value));
    b.y = Math.max(0, Math.round(sy + (ev.clientY - py) / scale.value));
  };
  const up = (ev: PointerEvent) => {
    el.releasePointerCapture(ev.pointerId);
    el.removeEventListener("pointermove", move);
    el.removeEventListener("pointerup", up);
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
  const move = (ev: PointerEvent) => {
    b.w = Math.max(40, Math.round(sw + (ev.clientX - px) / scale.value));
    b.h = Math.max(40, Math.round(sh + (ev.clientY - py) / scale.value));
  };
  const up = (ev: PointerEvent) => {
    el.releasePointerCapture(ev.pointerId);
    el.removeEventListener("pointermove", move);
    el.removeEventListener("pointerup", up);
    commit();
  };
  el.addEventListener("pointermove", move);
  el.addEventListener("pointerup", up);
}
</script>

<template>
  <div class="slide-editor">
    <div class="slide-editor-body">
      <div class="slide-toolrail">
        <span class="slide-toolrail-label">{{ t('workspace.storage.slide.add') }}</span>
        <button
          v-for="k in kinds" :key="k.name" type="button" class="slide-tool"
          :title="t(k.label_key)" @click="addBlock(k.name)"
        >
          <i class="fa-solid" :class="iconFor(k.name)" aria-hidden="true"></i>
          <span>{{ t(k.label_key) }}</span>
        </button>
      </div>

      <div ref="stageWrap" class="slide-stage-wrap">
        <div class="slide-stage-spacer" :style="wrapSpacerStyle">
          <div class="slide-stage" :style="stageStyle" @pointerdown.self="selectedId = null; editingId = null">
            <div
              v-for="b in blocks" :key="b.id"
              class="slide-block-box"
              :class="{ selected: b.id === selectedId, editing: b.id === editingId }"
              :style="blockStyle(b)"
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
          </div>
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
            <label>W<input type="number" v-model.number="selected.w" @change="commit" /></label>
            <label>H<input type="number" v-model.number="selected.h" @change="commit" /></label>
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

          <SlideElementTransition :block="selected" @patch="applyPatch(selected, $event)" />

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
