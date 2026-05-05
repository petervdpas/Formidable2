<script setup lang="ts">
import { useToast } from "../composables/useToast";

const { toasts, dismiss } = useToast();
</script>

<template>
  <Teleport to="body">
    <TransitionGroup name="toast" tag="div" class="toast-container">
      <div
        v-for="t in toasts"
        :key="t.id"
        :class="['toast', t.variant]"
        role="status"
        aria-live="polite"
        @click="dismiss(t.id)"
      >
        {{ t.text }}
      </div>
    </TransitionGroup>
  </Teleport>
</template>
