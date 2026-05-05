<script setup lang="ts">
import Modal from "./Modal.vue";

withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    message?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    variant?: "default" | "danger";
  }>(),
  {
    confirmLabel: "OK",
    cancelLabel: "Cancel",
    variant: "default",
  },
);

const emit = defineEmits<{
  (e: "confirm"): void;
  (e: "cancel"): void;
}>();
</script>

<template>
  <Modal :open="open" :title="title" @close="emit('cancel')">
    <p v-if="message" class="confirm-message">{{ message }}</p>
    <slot />

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ cancelLabel }}
      </button>
      <button
        class="tool-btn"
        :class="variant === 'danger' ? 'danger' : 'primary'"
        type="button"
        @click="emit('confirm')"
      >
        {{ confirmLabel }}
      </button>
    </template>
  </Modal>
</template>
