<script setup lang="ts">
// Reveal "mermaid" element: reuses the standard mermaid field editor; the
// canvas shows the backend-rendered diagram.
import { computed } from "vue";
import FormFieldMermaid from "../FormFieldMermaid.vue";
import SlideRenderedPreview from "./SlideRenderedPreview.vue";
import { syntheticField, type SlideBlock } from "../../../types/slide-blocks";

const props = defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const field = computed(() => syntheticField(props.block.id, "mermaid"));
</script>

<template>
  <SlideRenderedPreview v-if="surface === 'canvas'" :block="block" :html="html" />
  <FormFieldMermaid
    v-else :field="field" :model-value="block.content"
    @update:model-value="emit('patch', { content: $event })"
  />
</template>
