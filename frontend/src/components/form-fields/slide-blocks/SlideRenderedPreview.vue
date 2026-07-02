<script setup lang="ts">
// The canvas preview box for kinds whose on-slide look is just the backend
// render (image, table, list, mermaid, video). Text-like kinds edit inline
// instead (SlideInlineText); embed needs an eager iframe (SlideEmbed).
import RenderedHtml from "../../RenderedHtml.vue";
import type { SlideBlock } from "../../../types/slide-blocks";

defineProps<{ block: SlideBlock; html?: string }>();
</script>

<template>
  <div class="slide-block-box-content formidable-prose" :style="block.style ?? {}">
    <div class="slide-fit"><RenderedHtml :html="html ?? ''" /></div>
  </div>
</template>
