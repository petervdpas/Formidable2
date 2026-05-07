<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { ShapeInfo } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
}>();

const emit = defineEmits<{
  (e: "confirm", shape: string): void;
  (e: "cancel"): void;
}>();

const { t } = useI18n();

const shapes = ref<ShapeInfo[]>([]);
const selected = ref<string>("report");
const loading = ref(false);

// Lazy-load the catalog when the dialog opens. The catalog is static,
// so a single fetch per session would be enough — but the Vue layer
// has no global cache for this and the call is cheap.
watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    loading.value = true;
    try {
      const list = await TemplateSvc.GeneratorShapes();
      shapes.value = list ?? [];
      if (shapes.value.length && !shapes.value.some((s) => s.id === selected.value)) {
        selected.value = shapes.value[0].id ?? "report";
      }
    } finally {
      loading.value = false;
    }
  },
  { immediate: true },
);

function onConfirm() {
  emit("confirm", selected.value);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.generate.title')"
    width="540px"
    @close="emit('cancel')"
  >
    <p class="muted small generate-intro">
      {{ t('workspace.templates.generate.description') }}
    </p>

    <p v-if="loading" class="muted small">{{ t('common.loading') }}</p>

    <div v-else class="generate-shape-list" role="radiogroup">
      <label
        v-for="shape in shapes"
        :key="shape.id"
        class="generate-shape-row"
        :class="{ selected: selected === shape.id }"
      >
        <input
          type="radio"
          name="generate-shape"
          :value="shape.id"
          v-model="selected"
        />
        <span class="generate-shape-text">
          <span class="generate-shape-label">{{ shape.label }}</span>
          <span class="generate-shape-desc muted small">{{ shape.description }}</span>
        </span>
      </label>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="loading || !selected"
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
