<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { useToast, type Toast } from "../composables/useToast";

const { t } = useI18n();
const { toasts, dismiss } = useToast();

function runAction(toast: Toast): void {
  toast.action?.run();
  dismiss(toast.id);
}
</script>

<template>
  <Teleport to="body">
    <TransitionGroup name="toast" tag="div" class="toast-container">
      <div
        v-for="toast in toasts"
        :key="toast.id"
        :class="['toast', toast.variant]"
        role="status"
        aria-live="polite"
        @click="dismiss(toast.id)"
      >
        <span class="toast-text">{{ toast.text }}</span>
        <button
          v-if="toast.action"
          type="button"
          class="toast-action"
          @click.stop="runAction(toast)"
        >{{ toast.action.label }}</button>
        <button
          type="button"
          class="toast-close"
          :aria-label="t('common.close')"
          :title="t('common.close')"
          @click.stop="dismiss(toast.id)"
        >×</button>
      </div>
    </TransitionGroup>
  </Teleport>
</template>
