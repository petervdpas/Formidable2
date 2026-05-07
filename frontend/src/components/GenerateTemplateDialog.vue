<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type {
  ShapeInfo,
  ImgModeInfo,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
}>();

const emit = defineEmits<{
  (e: "confirm", shape: string, imgMode: string): void;
  (e: "cancel"): void;
}>();

const { t } = useI18n();

const shapes = ref<ShapeInfo[]>([]);
const imgModes = ref<ImgModeInfo[]>([]);
const selectedShape = ref<string>("report");
const selectedImgMode = ref<string>("url");
const loading = ref(false);

// Lazy-load the two catalogs when the dialog opens. Both are static
// (driven by Go constants) so we could cache once per session — but
// the calls are cheap and Vue doesn't have a global cache helper here.
watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    loading.value = true;
    try {
      const [shapeList, modeList] = await Promise.all([
        TemplateSvc.GeneratorShapes(),
        TemplateSvc.GeneratorImageModes(),
      ]);
      shapes.value = shapeList ?? [];
      imgModes.value = modeList ?? [];
      if (shapes.value.length && !shapes.value.some((s) => s.id === selectedShape.value)) {
        selectedShape.value = shapes.value[0].id ?? "report";
      }
      if (imgModes.value.length && !imgModes.value.some((m) => m.id === selectedImgMode.value)) {
        selectedImgMode.value = imgModes.value[0].id ?? "url";
      }
    } finally {
      loading.value = false;
    }
  },
  { immediate: true },
);

function onConfirm() {
  emit("confirm", selectedShape.value, selectedImgMode.value);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.generate.title')"
    width="600px"
    @close="emit('cancel')"
  >
    <p class="muted small generate-intro">
      {{ t('workspace.templates.generate.description') }}
    </p>

    <p v-if="loading" class="muted small">{{ t('common.loading') }}</p>

    <template v-else>
      <fieldset class="generate-fieldset">
        <legend>{{ t('workspace.templates.generate.shape_legend') }}</legend>
        <div class="generate-shape-list" role="radiogroup">
          <label
            v-for="shape in shapes"
            :key="shape.id"
            class="generate-shape-row"
            :class="{ selected: selectedShape === shape.id }"
          >
            <input
              type="radio"
              name="generate-shape"
              :value="shape.id"
              v-model="selectedShape"
            />
            <span class="generate-shape-text">
              <span class="generate-shape-label">{{ shape.label }}</span>
              <span class="generate-shape-desc muted small">{{ shape.description }}</span>
            </span>
          </label>
        </div>
      </fieldset>

      <fieldset class="generate-fieldset">
        <legend>{{ t('workspace.templates.generate.imgmode_legend') }}</legend>
        <div class="generate-shape-list" role="radiogroup">
          <label
            v-for="mode in imgModes"
            :key="mode.id"
            class="generate-shape-row"
            :class="{ selected: selectedImgMode === mode.id }"
          >
            <input
              type="radio"
              name="generate-imgmode"
              :value="mode.id"
              v-model="selectedImgMode"
            />
            <span class="generate-shape-text">
              <span class="generate-shape-label">{{ mode.label }}</span>
              <span class="generate-shape-desc muted small">{{ mode.description }}</span>
            </span>
          </label>
        </div>
      </fieldset>
    </template>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="loading || !selectedShape || !selectedImgMode"
        @click="onConfirm"
      >
        {{ t('workspace.templates.generate.confirm') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.generate-intro {
  margin: 0 0 0.75rem 0;
}
.generate-fieldset {
  border: 1px solid var(--border-color, #ccc);
  border-radius: 6px;
  padding: 0.5rem 0.75rem 0.75rem;
  margin: 0 0 0.75rem 0;
}
.generate-fieldset legend {
  padding: 0 0.4rem;
  font-weight: 600;
  font-size: 0.85rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--muted-color, #888);
}
.generate-shape-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.generate-shape-row {
  display: flex;
  align-items: flex-start;
  gap: 0.6rem;
  padding: 0.55rem 0.7rem;
  border: 1px solid var(--border-color, #ccc);
  border-radius: 6px;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.generate-shape-row:hover {
  border-color: var(--accent-color, #4a90e2);
}
.generate-shape-row.selected {
  border-color: var(--accent-color, #4a90e2);
  background: var(--accent-bg, rgba(74, 144, 226, 0.08));
}
.generate-shape-row input[type="radio"] {
  margin-top: 0.2rem;
}
.generate-shape-text {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}
.generate-shape-label {
  font-weight: 600;
}
.generate-shape-desc {
  line-height: 1.35;
}
</style>
