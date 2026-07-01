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
const FRAGMENT_OPTIONS = [
  "", "fade-in", "fade-up", "fade-down", "fade-left", "fade-right",
  "grow", "shrink", "strike", "highlight-red", "highlight-green", "highlight-blue",
];
const TRANSITION_OPTIONS = ["", "none", "fade", "slide", "convex", "concave", "zoom"];
const URL_KINDS = new Set(["video", "embed"]);

// ── doc state ─────────────────────────────────────────────────────────
const parsed = parseSlideDoc(props.modelValue);
const blocks = ref<SlideBlock[]>(parsed.blocks);
const background = ref(parsed.background ?? "");
const transition = ref(parsed.transition ?? "");
const notes = ref(parsed.notes ?? "");
const selectedId = ref<string | null>(null);
const editingId = ref<string | null>(null); // block being edited inline on the canvas

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
const selectedField = computed(() => (selected.value ? fieldForBlock(selected.value) : null));

function setContent(v: unknown) {
  if (selected.value) {
    selected.value.content = v;
    renderBlock(selected.value);
    commit();
  }
}
// inline typing on the canvas
function setInline(b: SlideBlock, v: string) {
  b.content = v;
  renderBlock(b);
  commit();
}
function startEdit(b: SlideBlock) {
  if (!INLINE_TEXT_KINDS.has(b.kind)) return;
  selectedId.value = b.id;
  editingId.value = b.id;
}

// ── per-element style (stored in the blob, applied inline) ──────────────
function styleVal(prop: string): string {
  return selected.value?.style?.[prop] ?? "";
}
function setStyle(prop: string, val: string) {
  if (!selected.value) return;
  const s = { ...(selected.value.style ?? {}) };
  if (val) s[prop] = val;
  else delete s[prop];
  selected.value.style = s;
  commit();
}
const fontSize = computed(() => parseInt(styleVal("font-size"), 10) || 40);
const isBold = computed(() => styleVal("font-weight") === "bold");
function blockContentStyle(b: SlideBlock) {
  return (b.style ?? {}) as Record<string, string>;
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
              <textarea
                v-if="b.id === editingId"
                class="slide-inline-edit"
                :style="blockContentStyle(b)"
                :value="String(b.content ?? '')"
                :ref="(el) => { if (el) (el as HTMLTextAreaElement).focus(); }"
                @input="setInline(b, ($event.target as HTMLTextAreaElement).value)"
                @blur="editingId = null"
                @pointerdown.stop
                @dblclick.stop
              ></textarea>
              <div v-else class="slide-block-box-content formidable-prose" :style="blockContentStyle(b)">
                <RenderedHtml :html="blockHtml[b.id] ?? ''" />
              </div>
              <span v-if="b.fragment" class="slide-block-box-frag">{{ b.fragment }}</span>
              <span v-if="b.id === selectedId" class="slide-block-box-dim">{{ b.w }} × {{ b.h }}</span>
              <div class="slide-block-box-resize" @pointerdown="startResize(b, $event)"></div>
            </div>
          </div>
        </div>
      </div>

      <aside class="slide-inspector">
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

          <!-- content: specialised editors for structured kinds; text-like kinds
               are typed inline on the canvas (double-click). -->
          <div class="slide-inspector-content">
            <FormFieldRenderer
              v-if="selectedField"
              :field="selectedField" :model-value="selected.content"
              @update:model-value="setContent"
            />
            <input
              v-else-if="URL_KINDS.has(selected.kind)" type="text" class="slide-url-input"
              placeholder="https://…" :value="String(selected.content ?? '')"
              @input="setContent(($event.target as HTMLInputElement).value)"
            />
            <template v-else-if="INLINE_TEXT_KINDS.has(selected.kind)">
              <input
                v-if="selected.kind === 'code'" type="text" class="slide-lang-input"
                :placeholder="t('workspace.storage.slide.code_lang')" :value="selected.lang ?? ''"
                @input="selected.lang = ($event.target as HTMLInputElement).value; commit()"
              />
              <textarea
                class="slide-prop-text" rows="6"
                :class="{ 'is-mono': selected.kind === 'code' }"
                :value="String(selected.content ?? '')"
                @input="setContent(($event.target as HTMLTextAreaElement).value)"
              ></textarea>
              <p class="muted small">{{ t('workspace.storage.slide.edit_inline') }}</p>
            </template>
          </div>

          <!-- per-element style, stored in the block -->
          <div class="slide-style-grid">
            <label>{{ t('workspace.storage.slide.font_size') }}
              <input type="number" min="8" :value="fontSize" @input="setStyle('font-size', (($event.target as HTMLInputElement).value || '40') + 'px')" />
            </label>
            <label>{{ t('workspace.storage.slide.color') }}
              <input type="color" :value="styleVal('color') || '#000000'" @input="setStyle('color', ($event.target as HTMLInputElement).value)" />
            </label>
            <div class="slide-style-align">
              <button type="button" :class="{ active: styleVal('text-align') === 'left' }" @click="setStyle('text-align', 'left')"><i class="fa-solid fa-align-left"></i></button>
              <button type="button" :class="{ active: styleVal('text-align') === 'center' }" @click="setStyle('text-align', 'center')"><i class="fa-solid fa-align-center"></i></button>
              <button type="button" :class="{ active: styleVal('text-align') === 'right' }" @click="setStyle('text-align', 'right')"><i class="fa-solid fa-align-right"></i></button>
              <button type="button" :class="{ active: isBold }" @click="setStyle('font-weight', isBold ? '' : 'bold')"><i class="fa-solid fa-bold"></i></button>
            </div>
          </div>

          <label class="slide-inspector-row">
            {{ t('workspace.storage.slide.fragment') }}
            <select :value="selected.fragment ?? ''" @change="selected.fragment = ($event.target as HTMLSelectElement).value; commit()">
              <option v-for="f in FRAGMENT_OPTIONS" :key="f" :value="f">{{ f || '—' }}</option>
            </select>
          </label>

          <button type="button" class="tool-btn danger" @click="removeSelected">
            {{ t('workspace.storage.slide.delete_block') }}
          </button>
        </template>
        <p v-else class="muted small">{{ t('workspace.storage.slide.no_selection') }}</p>

        <hr class="slide-inspector-sep" />
        <div class="slide-inspector-head">{{ t('workspace.storage.slide.slide_settings') }}</div>
        <label class="slide-inspector-row">
          {{ t('workspace.storage.slide.background') }}
          <input type="color" :value="background || '#ffffff'" @input="background = ($event.target as HTMLInputElement).value; commit()" />
        </label>
        <label class="slide-inspector-row">
          {{ t('workspace.storage.slide.transition') }}
          <select :value="transition" @change="transition = ($event.target as HTMLSelectElement).value; commit()">
            <option v-for="tr in TRANSITION_OPTIONS" :key="tr" :value="tr">{{ tr || '—' }}</option>
          </select>
        </label>
        <label class="slide-inspector-col">
          {{ t('workspace.storage.slide.notes') }}
          <textarea rows="3" :value="notes" @input="notes = ($event.target as HTMLTextAreaElement).value; commit()"></textarea>
        </label>
      </aside>
    </div>
  </div>
</template>
