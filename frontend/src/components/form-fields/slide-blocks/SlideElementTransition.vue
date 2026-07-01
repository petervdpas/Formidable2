<script setup lang="ts">
// Element-level transition: a reveal fragment animation applied to this block
// when the slide advances (distinct from the slide-level transition in
// SlideSettings). Stored as block.fragment (the reveal fragment class).
import { useI18n } from "vue-i18n";
import type { SlideBlock } from "../../../types/slide-blocks";

defineProps<{ block: SlideBlock }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();
const FRAGMENT_OPTIONS = [
  "", "fade-in", "fade-up", "fade-down", "fade-left", "fade-right",
  "grow", "shrink", "strike", "highlight-red", "highlight-green", "highlight-blue",
];
</script>

<template>
  <label class="slide-inspector-row">
    {{ t('workspace.storage.slide.element_transition') }}
    <select :value="block.fragment ?? ''" @change="emit('patch', { fragment: ($event.target as HTMLSelectElement).value })">
      <option v-for="f in FRAGMENT_OPTIONS" :key="f" :value="f">{{ f || '—' }}</option>
    </select>
  </label>
</template>
