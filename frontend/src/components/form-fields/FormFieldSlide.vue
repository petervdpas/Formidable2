<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "../Modal.vue";
import SlideCanvasEditor from "./SlideCanvasEditor.vue";
import { parseSlideDoc, canvasSize } from "../../types/slide-blocks";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

const open = ref(false);

const blockCount = computed(() => parseSlideDoc(props.modelValue).blocks.length);
const canvas = computed(() => canvasSize(props.field));
</script>

<template>
  <div class="slide-trigger">
    <button type="button" class="tool-btn" @click="open = true">
      <i class="fa-solid fa-object-group" aria-hidden="true"></i>
      {{ t('workspace.storage.slide.edit') }}
    </button>
    <span class="slide-trigger-status" :class="{ muted: blockCount === 0 }">
      {{ t('workspace.storage.slide.blocks_count', [blockCount]) }}
    </span>
  </div>

  <Modal
    :open="open"
    :title="t('workspace.storage.slide.title')"
    width="1120px"
    :dialog-style="{ height: '82vh' }"
    maximizable
    fill
    @close="open = false"
  >
    <SlideCanvasEditor
      :model-value="modelValue"
      :canvas-w="canvas.w"
      :canvas-h="canvas.h"
      @update:model-value="(v: unknown) => emit('update:modelValue', v)"
    />
  </Modal>
</template>
