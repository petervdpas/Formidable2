<script setup lang="ts">
import { onBeforeUnmount, watch, nextTick, useTemplateRef } from "vue";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    closeOnBackdrop?: boolean;
    closeOnEsc?: boolean;
    /** Width override for the dialog box (CSS value, e.g. "480px"). */
    width?: string;
    /** Optional class added to the dialog (e.g. for type-tinting). */
    dialogClass?: string;
    /** Optional inline style merged into the dialog (e.g. CSS vars). */
    dialogStyle?: Record<string, string>;
  }>(),
  {
    closeOnBackdrop: true,
    closeOnEsc: true,
    width: "480px",
  },
);

const emit = defineEmits<{ (e: "close"): void }>();

const dialogRef = useTemplateRef<HTMLDivElement>("dialog");

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape" && props.closeOnEsc && props.open) {
    e.stopPropagation();
    emit("close");
  }
}

function onBackdropClick() {
  if (props.closeOnBackdrop) emit("close");
}

watch(
  () => props.open,
  async (isOpen) => {
    if (isOpen) {
      window.addEventListener("keydown", onKeydown, { capture: true });
      await nextTick();
      // Focus the first input in the dialog body, falling back to the
      // dialog itself so Esc still works.
      const root = dialogRef.value;
      const target =
        (root?.querySelector<HTMLElement>(
          "input, textarea, select, button:not([data-modal-close])",
        )) ?? root;
      target?.focus();
    } else {
      window.removeEventListener("keydown", onKeydown, { capture: true });
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKeydown, { capture: true });
});
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="open" class="modal-backdrop" @click.self="onBackdropClick">
        <div
          ref="dialog"
          :class="['modal-dialog', dialogClass]"
          :style="{ width, ...(dialogStyle || {}) }"
          role="dialog"
          aria-modal="true"
          :aria-label="title"
          tabindex="-1"
        >
          <header v-if="title || $slots.title" class="modal-header">
            <h2 class="modal-title">
              <slot name="title">{{ title }}</slot>
            </h2>
            <button
              class="modal-close"
              type="button"
              data-modal-close
              aria-label="Close"
              @click="emit('close')"
            >×</button>
          </header>

          <div class="modal-body">
            <slot />
          </div>

          <footer v-if="$slots.footer" class="modal-footer">
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
