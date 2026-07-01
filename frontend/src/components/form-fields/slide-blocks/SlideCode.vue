<script setup lang="ts">
// Reveal "code" element: a fenced, syntax-highlighted code block. Adds a
// language input above the shared inline text editor.
import { useI18n } from "vue-i18n";
import SlideInlineText from "./SlideInlineText.vue";
import type { SlideBlock } from "../../../types/slide-blocks";

defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string; editing?: boolean }>();
const emit = defineEmits<{
  (e: "patch", p: Partial<SlideBlock>): void;
  (e: "end-edit"): void;
}>();

const { t } = useI18n();
</script>

<template>
  <template v-if="surface === 'inspector'">
    <input
      type="text" class="slide-lang-input"
      :placeholder="t('workspace.storage.slide.code_lang')" :value="block.lang ?? ''"
      @input="emit('patch', { lang: ($event.target as HTMLInputElement).value })"
    />
    <SlideInlineText
      :block="block" surface="inspector" :mono="true"
      hint-key="workspace.storage.slide.edit_inline"
      @patch="emit('patch', $event)"
    />
  </template>
  <SlideInlineText
    v-else
    :block="block" surface="canvas" :html="html" :editing="editing" :mono="true"
    @patch="emit('patch', $event)" @end-edit="emit('end-edit')"
  />
</template>
