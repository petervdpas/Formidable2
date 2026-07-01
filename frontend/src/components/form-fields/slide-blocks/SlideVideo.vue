<script setup lang="ts">
// Reveal "video" element: a media URL. The canvas shows the backend-rendered
// <video>; the inspector is a URL input.
import SlideRenderedPreview from "./SlideRenderedPreview.vue";
import type { SlideBlock } from "../../../types/slide-blocks";

defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();
</script>

<template>
  <SlideRenderedPreview v-if="surface === 'canvas'" :block="block" :html="html" />
  <input
    v-else type="text" class="slide-url-input" placeholder="https://…"
    :value="String(block.content ?? '')"
    @input="emit('patch', { content: ($event.target as HTMLInputElement).value })"
  />
</template>
