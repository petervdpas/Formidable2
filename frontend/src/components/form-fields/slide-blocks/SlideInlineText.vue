<script setup lang="ts">
// Shared presentational primitive for the text-like reveal kinds (text, quote,
// math, code): a slide-scale inline editor on the canvas (double-click to type,
// slides.com style) and, in the inspector, a textarea plus the element's own
// typographic property controls. All edits flow out as block patches.
import { useI18n } from "vue-i18n";
import RenderedHtml from "../../RenderedHtml.vue";
import SlideStyleControls from "./SlideStyleControls.vue";
import type { SlideBlock } from "../../../types/slide-blocks";

defineProps<{
  block: SlideBlock;
  surface: "canvas" | "inspector";
  html?: string;
  editing?: boolean;
  mono?: boolean;
  hintKey?: string;
}>();
const emit = defineEmits<{
  (e: "patch", p: Partial<SlideBlock>): void;
  (e: "end-edit"): void;
}>();

const { t } = useI18n();
</script>

<template>
  <template v-if="surface === 'canvas'">
    <textarea
      v-if="editing"
      class="slide-inline-edit"
      :class="{ 'is-mono': mono }"
      :style="block.style ?? {}"
      :value="String(block.content ?? '')"
      :ref="(el) => { if (el) (el as HTMLTextAreaElement).focus(); }"
      @input="emit('patch', { content: ($event.target as HTMLTextAreaElement).value })"
      @blur="emit('end-edit')"
      @pointerdown.stop
      @dblclick.stop
    ></textarea>
    <div v-else class="slide-block-box-content formidable-prose" :style="block.style ?? {}">
      <div class="slide-fit"><RenderedHtml :html="html ?? ''" /></div>
    </div>
  </template>

  <template v-else>
    <textarea
      class="slide-prop-text" rows="6"
      :class="{ 'is-mono': mono }"
      :value="String(block.content ?? '')"
      @input="emit('patch', { content: ($event.target as HTMLTextAreaElement).value })"
    ></textarea>
    <p v-if="hintKey" class="muted small">{{ t(hintKey) }}</p>
    <SlideStyleControls :block="block" @patch="emit('patch', $event)" />
  </template>
</template>
