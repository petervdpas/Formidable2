<script setup lang="ts">
// Element rotation: a per-block clockwise angle in degrees (block.rotation,
// 0/undefined = none). A slider and a matched number field edit the same value;
// the reset button snaps back to 0. The canvas and the deck both apply it as a
// transform:rotate() about the box centre, so what you tilt is what renders.
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { SlideBlock } from "../../../types/slide-blocks";

const props = defineProps<{ block: SlideBlock }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();
const angle = computed(() => props.block.rotation ?? 0);

// Keep the stored angle in [-180, 180] so the slider stays centred on 0 and a
// bad value can't drift out of range.
function set(v: number): void {
  const n = Number.isFinite(v) ? Math.round(v) : 0;
  const clamped = Math.max(-180, Math.min(180, n));
  emit("patch", { rotation: clamped });
}
</script>

<template>
  <div class="slide-inspector-row slide-rotation">
    <span>{{ t('workspace.storage.slide.rotation') }}</span>
    <div class="slide-rotation-controls">
      <input
        type="range" min="-180" max="180" step="1" :value="angle"
        @input="set(Number(($event.target as HTMLInputElement).value))"
      />
      <input
        type="number" min="-180" max="180" step="1" :value="angle"
        @change="set(Number(($event.target as HTMLInputElement).value))"
      />
      <button
        type="button" class="tool-btn" :disabled="angle === 0"
        :title="t('workspace.storage.slide.rotation_reset')" @click="set(0)"
      >
        <i class="fa-solid fa-rotate-left" aria-hidden="true"></i>
      </button>
    </div>
  </div>
</template>
