<script setup lang="ts">
// Reveal "text" element: markdown typed inline on the slide.
import SlideInlineText from "./SlideInlineText.vue";
import type { SlideBlock } from "../../../types/slide-blocks";

defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string; editing?: boolean }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void; (e: "end-edit"): void }>();
</script>

<template>
  <SlideInlineText
    :block="block" :surface="surface" :html="html" :editing="editing"
    hint-key="workspace.storage.slide.edit_inline"
    @patch="emit('patch', $event)" @end-edit="emit('end-edit')"
  />
</template>
