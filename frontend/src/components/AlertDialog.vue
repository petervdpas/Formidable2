<script setup lang="ts">
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";

withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    message?: string;
    okLabel?: string;
    variant?: "default" | "danger";
  }>(),
  {
    variant: "default",
  },
);

const emit = defineEmits<{ (e: "close"): void }>();
const { t } = useI18n();
</script>

<template>
  <Modal :open="open" :title="title ?? t('common.alert_title')" @close="emit('close')">
    <p v-if="message" class="confirm-message">{{ message }}</p>
    <slot />

    <template #footer>
      <button
        class="tool-btn"
        :class="variant === 'danger' ? 'danger' : 'primary'"
        type="button"
        @click="emit('close')"
      >
        {{ okLabel ?? t('common.ok') }}
      </button>
    </template>
  </Modal>
</template>
