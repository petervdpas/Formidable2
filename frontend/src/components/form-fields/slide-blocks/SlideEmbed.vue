<script setup lang="ts">
// Reveal "embed" element: an iframed URL. The deck lazy-loads it via reveal's
// data-src, which never fires in the editor, so the canvas renders an eager
// iframe preview (with a placeholder when empty). The inspector is a URL input.
import { useI18n } from "vue-i18n";
import type { SlideBlock } from "../../../types/slide-blocks";

defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();
</script>

<template>
  <div v-if="surface === 'canvas'" class="slide-block-box-content formidable-prose" :style="block.style ?? {}">
    <div class="slide-fit">
      <iframe
        v-if="block.content" class="slide-embed-preview"
        :src="String(block.content)" referrerpolicy="no-referrer"
      ></iframe>
      <div v-else class="slide-embed-fallback">
        <i class="fa-solid fa-window-maximize" aria-hidden="true"></i>
        <span>{{ t('workspace.storage.slide.embed_empty') }}</span>
      </div>
    </div>
  </div>
  <input
    v-else type="text" class="slide-url-input" placeholder="https://…"
    :value="String(block.content ?? '')"
    @input="emit('patch', { content: ($event.target as HTMLInputElement).value })"
  />
</template>
