<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import SwitchField from "./fields/SwitchField.vue";
import {
  Service as TemplateSvc,
  GeneratorOptions,
  ImgMode,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { ShapeInfo } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
}>();

const emit = defineEmits<{
  (e: "confirm", shape: string, opts: GeneratorOptions): void;
  (e: "cancel"): void;
}>();

const { t } = useI18n();

const shapes = ref<ShapeInfo[]>([]);
const selectedShape = ref<string>("report");

// Options section - booleans rather than radios so the dialog stays
// scannable when more options land later. Defaults match the backend
// defaults: linked URL for images, auto-wrap for loops, lazy api-card
// (one-liner per api field).
const inlineImages = ref(false);
const wrapLoops = ref(true);
const expandAPI = ref(false);

const loading = ref(false);

watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    loading.value = true;
    try {
      const list = await TemplateSvc.GeneratorShapes();
      shapes.value = list ?? [];
      if (shapes.value.length && !shapes.value.some((s) => s.id === selectedShape.value)) {
        selectedShape.value = shapes.value[0].id ?? "report";
      }
    } finally {
      loading.value = false;
    }
  },
  { immediate: true },
);

function onConfirm() {
  const opts = GeneratorOptions.createFrom({
    img_mode: inlineImages.value ? ImgMode.ImgInline : ImgMode.ImgURL,
    wrap_loops: wrapLoops.value,
    expand_api: expandAPI.value,
  });
  emit("confirm", selectedShape.value, opts);
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
        <legend>{{ t('workspace.templates.generate.options_legend') }}</legend>

        <div class="generate-option-row">
          <div class="generate-option-text">
            <span class="generate-option-label">
              {{ t('workspace.templates.generate.inline_images.label') }}
            </span>
            <span class="generate-option-desc muted small">
              {{ inlineImages
                ? t('workspace.templates.generate.inline_images.desc_on')
                : t('workspace.templates.generate.inline_images.desc_off') }}
            </span>
          </div>
          <SwitchField
            v-model="inlineImages"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </div>

        <div class="generate-option-row">
          <div class="generate-option-text">
            <span class="generate-option-label">
              {{ t('workspace.templates.generate.wrap_loops.label') }}
            </span>
            <span class="generate-option-desc muted small">
              {{ wrapLoops
                ? t('workspace.templates.generate.wrap_loops.desc_on')
                : t('workspace.templates.generate.wrap_loops.desc_off') }}
            </span>
          </div>
          <SwitchField
            v-model="wrapLoops"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </div>

        <div class="generate-option-row">
          <div class="generate-option-text">
            <span class="generate-option-label">
              {{ t('workspace.templates.generate.expand_api.label') }}
            </span>
            <span class="generate-option-desc muted small">
              {{ expandAPI
                ? t('workspace.templates.generate.expand_api.desc_on')
                : t('workspace.templates.generate.expand_api.desc_off') }}
            </span>
          </div>
          <SwitchField
            v-model="expandAPI"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
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
        :disabled="loading || !selectedShape"
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
.generate-option-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.5rem 0.2rem;
  border-top: 1px solid var(--border-color-faint, rgba(0, 0, 0, 0.05));
}
.generate-option-row:first-of-type {
  border-top: none;
}
.generate-option-text {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}
.generate-option-label {
  font-weight: 600;
}
.generate-option-desc {
  line-height: 1.35;
}
</style>
