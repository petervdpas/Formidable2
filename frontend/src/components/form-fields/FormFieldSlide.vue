<script setup lang="ts">
import { computed } from "vue";
import SlideCanvasEditor from "./SlideCanvasEditor.vue";
import { canvasSize } from "../../types/slide-blocks";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const canvas = computed(() => canvasSize(props.field));
</script>

<template>
  <div class="slide-field-inline">
    <SlideCanvasEditor
      :model-value="modelValue"
      :canvas-w="canvas.w"
      :canvas-h="canvas.h"
      @update:model-value="(v: unknown) => emit('update:modelValue', v)"
    />
  </div>
</template>
