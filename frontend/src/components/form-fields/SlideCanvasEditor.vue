<script setup lang="ts">
import { computed, inject, onMounted, ref, watch, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import FormFieldRenderer from "./FormFieldRenderer.vue";
import RenderedHtml from "../RenderedHtml.vue";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import {
  ensureSlideBlockKindsLoaded,
  slideBlockKinds,
  parseSlideDoc,
  newBlock,
  fieldForBlock,
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

// The template's YAML filename, provided by StorageWorkspace - scopes block
// image URLs when rendering.
const templateFilename = inject<ComputedRef<string>>(
  "templateFilename",
  computed(() => ""),
);

const blocks = ref<SlideBlock[]>(parseSlideDoc(props.modelValue).blocks);
const selectedId = ref<string | null>(null);

// Rendered HTML per block, so each box shows its real content (the same output
// the deck will render). Kept in a map keyed by block id.
const blockHtml = ref<Record<string, string>>({});
const renderTimers: Record<string, ReturnType<typeof setTimeout>> = {};

// renderBlock re-renders one block's content, debounced so typing in the
// inspector doesn't spam the backend. Geometry changes (drag/resize) never call
// this - the box and its content just move via CSS.
function renderBlock(b: SlideBlock) {
  clearTimeout(renderTimers[b.id]);
  renderTimers[b.id] = setTimeout(async () => {
    blockHtml.value[b.id] = await RenderSvc.RenderSlideBlockHTML(
      templateFilename.value,
      b.kind,
      b.content,
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

// External resets (undo, reload) replace the local blocks; guarded by a JSON
// compare so our own commits don't bounce back through here.
watch(
  () => props.modelValue,
  (v) => {
    const incoming = parseSlideDoc(v).blocks;
    if (JSON.stringify(incoming) !== JSON.stringify(blocks.value)) {
      blocks.value = incoming;
      if (!blocks.value.some((b) => b.id === selectedId.value)) selectedId.value = null;
      renderAll();
    }
  },
);

function commit() {
  emit("update:modelValue", { blocks: JSON.parse(JSON.stringify(blocks.value)) });
}

// ── palette ───────────────────────────────────────────────────────────
const kinds = computed(() => slideBlockKinds());
const labelFor = (kind: string) =>
  t(kinds.value.find((k) => k.name === kind)?.label_key ?? kind);

const KIND_ICON: Record<string, string> = {
  textarea: "fa-paragraph",
  mermaid: "fa-diagram-project",
  image: "fa-image",
  table: "fa-table",
  list: "fa-list-ul",
};
const iconFor = (kind: string) => KIND_ICON[kind] ?? "fa-square";

function addBlock(kind: string) {
  const b = newBlock(kind, blocks.value.length);
  blocks.value.push(b);
  selectedId.value = b.id;
  renderBlock(b);
  commit();
}

function removeSelected() {
  blocks.value = blocks.value.filter((b) => b.id !== selectedId.value);
  selectedId.value = null;
  commit();
}

const selected = computed(() => blocks.value.find((b) => b.id === selectedId.value) ?? null);
const selectedField = computed(() => (selected.value ? fieldForBlock(selected.value) : null));

function setContent(v: unknown) {
  if (selected.value) {
    selected.value.content = v;
    renderBlock(selected.value);
    commit();
  }
}

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
}));
const wrapSpacerStyle = computed(() => ({
  width: `${props.canvasW * scale.value}px`,
  height: `${props.canvasH * scale.value}px`,
}));

function blockStyle(b: SlideBlock) {
  return { left: `${b.x}px`, top: `${b.y}px`, width: `${b.w}px`, height: `${b.h}px` };
}

// ── drag / resize (pixel space, deltas scaled back to canvas units) ─────
function startDrag(b: SlideBlock, e: PointerEvent) {
  if (e.button !== 0) return;
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
          v-for="k in kinds"
          :key="k.name"
          type="button"
          class="slide-tool"
          :title="t(k.label_key)"
          @click="addBlock(k.name)"
        >
          <i class="fa-solid" :class="iconFor(k.name)" aria-hidden="true"></i>
          <span>{{ t(k.label_key) }}</span>
        </button>
      </div>

      <div ref="stageWrap" class="slide-stage-wrap">
        <div class="slide-stage-spacer" :style="wrapSpacerStyle">
          <div class="slide-stage" :style="stageStyle">
            <div
              v-for="b in blocks"
              :key="b.id"
              class="slide-block-box"
              :class="{ selected: b.id === selectedId }"
              :style="blockStyle(b)"
              @pointerdown="startDrag(b, $event)"
            >
              <div class="slide-block-box-content formidable-prose">
                <RenderedHtml :html="blockHtml[b.id] ?? ''" />
              </div>
              <span v-if="b.id === selectedId" class="slide-block-box-dim">
                {{ b.w }} × {{ b.h }}
              </span>
              <div class="slide-block-box-resize" @pointerdown="startResize(b, $event)"></div>
            </div>
          </div>
        </div>
      </div>

      <aside class="slide-inspector">
        <template v-if="selected && selectedField">
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
          <div class="slide-inspector-content">
            <FormFieldRenderer
              :field="selectedField"
              :model-value="selected.content"
              @update:model-value="setContent"
            />
          </div>
          <button type="button" class="tool-btn danger" @click="removeSelected">
            {{ t('workspace.storage.slide.delete_block') }}
          </button>
        </template>
        <p v-else class="muted small">{{ t('workspace.storage.slide.no_selection') }}</p>
      </aside>
    </div>
  </div>
</template>
