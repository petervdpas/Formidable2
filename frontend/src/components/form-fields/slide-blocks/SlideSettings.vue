<script setup lang="ts">
// Per-slide (reveal <section>) settings: background colour, transition override,
// and speaker notes. Kept separate from the block inspector so the deck-section
// attributes live in one component.
import { useI18n } from "vue-i18n";

defineProps<{ background: string; transition: string; notes: string }>();
const emit = defineEmits<{
  (e: "update:background", v: string): void;
  (e: "update:transition", v: string): void;
  (e: "update:notes", v: string): void;
}>();

const { t } = useI18n();
const TRANSITION_OPTIONS = ["", "none", "fade", "slide", "convex", "concave", "zoom"];
</script>

<template>
  <div class="slide-inspector-head">{{ t('workspace.storage.slide.slide_settings') }}</div>
  <label class="slide-inspector-row">
    {{ t('workspace.storage.slide.background') }}
    <input
      type="color" :value="background || '#ffffff'"
      @input="emit('update:background', ($event.target as HTMLInputElement).value)"
    />
  </label>
  <label class="slide-inspector-row">
    {{ t('workspace.storage.slide.transition') }}
    <select :value="transition" @change="emit('update:transition', ($event.target as HTMLSelectElement).value)">
      <option v-for="tr in TRANSITION_OPTIONS" :key="tr" :value="tr">{{ tr || '—' }}</option>
    </select>
  </label>
  <label class="slide-inspector-col">
    {{ t('workspace.storage.slide.notes') }}
    <textarea rows="3" :value="notes" @input="emit('update:notes', ($event.target as HTMLTextAreaElement).value)"></textarea>
  </label>
</template>
