<script setup lang="ts">
// Element-level shadow: a per-block preset (none/soft/medium/strong) stored as
// block.shadow, plus a direction (block.shadowDir, default down) shown only when
// a shadow is set. The stylesheet maps the preset to the right CSS per block kind
// (drop-shadow for image/shape/mermaid/math, box-shadow for table/code/video/
// embed, text-shadow for text/quote/list) and the direction to the offset vector.
import { computed, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import {
  ensureSlideShadowsLoaded,
  ensureSlideShadowDirectionsLoaded,
  slideShadows,
  slideShadowDirections,
  type SlideBlock,
} from "../../../types/slide-blocks";

const props = defineProps<{ block: SlideBlock }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();
const shadows = computed(() => slideShadows());
const directions = computed(() => slideShadowDirections());
const hasShadow = computed(() => !!props.block.shadow);

onMounted(() => {
  void ensureSlideShadowsLoaded();
  void ensureSlideShadowDirectionsLoaded();
});
</script>

<template>
  <label class="slide-inspector-row">
    {{ t('workspace.storage.slide.shadow') }}
    <select :value="block.shadow ?? ''" @change="emit('patch', { shadow: ($event.target as HTMLSelectElement).value })">
      <option v-for="s in shadows" :key="s.value" :value="s.value">{{ t(s.label_key) }}</option>
    </select>
  </label>
  <label v-if="hasShadow" class="slide-inspector-row">
    {{ t('workspace.storage.slide.shadow_dir') }}
    <select :value="block.shadowDir ?? ''" @change="emit('patch', { shadowDir: ($event.target as HTMLSelectElement).value })">
      <option v-for="d in directions" :key="d.value" :value="d.value">{{ t(d.label_key) }}</option>
    </select>
  </label>
</template>
