<script setup lang="ts">
// Element-level shadow: a per-block preset (none/soft/medium/strong) stored as
// block.shadow. The stylesheet maps the preset to the right CSS per block kind
// (drop-shadow for image/shape/mermaid/math, box-shadow for table/code/video/
// embed, text-shadow for text/quote/list), so this is just a preset picker.
import { computed, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import {
  ensureSlideShadowsLoaded,
  slideShadows,
  type SlideBlock,
} from "../../../types/slide-blocks";

defineProps<{ block: SlideBlock }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();
const shadows = computed(() => slideShadows());
onMounted(() => void ensureSlideShadowsLoaded());
</script>

<template>
  <label class="slide-inspector-row">
    {{ t('workspace.storage.slide.shadow') }}
    <select :value="block.shadow ?? ''" @change="emit('patch', { shadow: ($event.target as HTMLSelectElement).value })">
      <option v-for="s in shadows" :key="s.value" :value="s.value">{{ t(s.label_key) }}</option>
    </select>
  </label>
</template>
