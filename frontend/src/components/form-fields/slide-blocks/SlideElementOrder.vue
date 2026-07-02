<script setup lang="ts">
// Element stacking order (z-order). Blocks paint in document order, so "bring
// forward" / "send backward" move this block later / earlier in the deck. The
// parent owns the block array, so this control just emits intent.
import { useI18n } from "vue-i18n";

defineProps<{ canForward: boolean; canBackward: boolean }>();
const emit = defineEmits<{ (e: "forward"): void; (e: "backward"): void }>();

const { t } = useI18n();
</script>

<template>
  <div class="slide-inspector-row slide-order">
    <span>{{ t('workspace.storage.slide.order') }}</span>
    <div class="slide-order-btns">
      <button
        type="button" class="tool-btn" :disabled="!canForward"
        :title="t('workspace.storage.slide.bring_forward')" @click="emit('forward')"
      >
        <i class="fa-solid fa-arrow-up" aria-hidden="true"></i>
      </button>
      <button
        type="button" class="tool-btn" :disabled="!canBackward"
        :title="t('workspace.storage.slide.send_backward')" @click="emit('backward')"
      >
        <i class="fa-solid fa-arrow-down" aria-hidden="true"></i>
      </button>
    </div>
  </div>
</template>
