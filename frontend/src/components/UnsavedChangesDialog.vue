<script setup lang="ts">
import Modal from "./Modal.vue";

withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    message?: string;
    saveLabel?: string;
    discardLabel?: string;
    cancelLabel?: string;
  }>(),
  {
    saveLabel: "Save",
    discardLabel: "Discard",
    cancelLabel: "Cancel",
  },
);

const emit = defineEmits<{
  (e: "save"): void;
  (e: "discard"): void;
  (e: "cancel"): void;
}>();
</script>

<template>
  <Modal :open="open" :title="title" @close="emit('cancel')">
    <p v-if="message" class="confirm-message">{{ message }}</p>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ cancelLabel }}
      </button>
      <button class="tool-btn danger" type="button" @click="emit('discard')">
        {{ discardLabel }}
      </button>
      <button class="tool-btn primary" type="button" @click="emit('save')">
        {{ saveLabel }}
      </button>
    </template>
  </Modal>
</template>
