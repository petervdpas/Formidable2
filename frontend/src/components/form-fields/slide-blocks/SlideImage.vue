<script setup lang="ts">
// Reveal "image" element: reuses the standard image field for picking/storing
// the image; the canvas shows the backend-rendered <img>.
import { computed } from "vue";
import FormFieldImage from "../FormFieldImage.vue";
import SlideRenderedPreview from "./SlideRenderedPreview.vue";
import { syntheticField, type SlideBlock } from "../../../types/slide-blocks";

const props = defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const field = computed(() => syntheticField(props.block.id, "image"));
</script>

<template>
  <SlideRenderedPreview v-if="surface === 'canvas'" :block="block" :html="html" />
  <FormFieldImage
    v-else :field="field" :model-value="block.content"
    @update:model-value="emit('patch', { content: $event })"
  />
</template>
