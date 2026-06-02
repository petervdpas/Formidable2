<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch, nextTick, useTemplateRef } from "vue";

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
    /** When true, render an expand/restore button in the header that
     *  toggles the dialog between its caller-supplied size and the
     *  full viewport (minus the backdrop padding). */
    maximizable?: boolean;
    /** When true, the dialog body is a flex column: the #head and #foot
     *  slots stay pinned and the default slot scrolls between them. Use
     *  for long forms/tables so the controls and column headers don't
     *  scroll out of view. */
    scroll?: boolean;
    /** When true, the body is a flex column whose single child stretches
     *  to fill the dialog height (so content grows when maximized). Give
     *  the dialog a height via dialogStyle for the non-maximized case. */
    fill?: boolean;
  }>(),
  {
    closeOnBackdrop: true,
    closeOnEsc: true,
    width: "480px",
    maximizable: false,
    scroll: false,
    fill: false,
  },
);

const emit = defineEmits<{ (e: "close"): void }>();

const maximized = ref(false);

// When maximized, ignore the caller's width / dialogStyle.height and
// fill the available viewport space. Restoring snaps back to the
// caller's original sizing - no animation, no half-states.
const computedStyle = computed(() => {
  if (maximized.value) {
    return {
      width: "calc(100vw - var(--space-4) * 2)",
      height: "calc(100vh - var(--space-4) * 2)",
    };
  }
  return { width: props.width, ...(props.dialogStyle || {}) };
});

function toggleMax() {
  maximized.value = !maximized.value;
}

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
          "input, textarea, select, button:not([data-modal-close]):not([data-modal-max])",
        )) ?? root;
      target?.focus();
    } else {
      window.removeEventListener("keydown", onKeydown, { capture: true });
      // Reopen starts un-maximized - matches OS window behaviour.
      maximized.value = false;
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
          :class="['modal-dialog', dialogClass, { 'modal-scrolling': scroll }]"
          :style="computedStyle"
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
              v-if="maximizable"
              class="modal-max"
              type="button"
              data-modal-max
              :aria-label="maximized ? 'Restore' : 'Maximize'"
              :title="maximized ? 'Restore' : 'Maximize'"
              @click="toggleMax"
            >
              <i :class="maximized ? 'fa-solid fa-compress' : 'fa-solid fa-expand'"></i>
            </button>
            <button
              class="modal-close"
              type="button"
              data-modal-close
              aria-label="Close"
              @click="emit('close')"
            >×</button>
          </header>

          <div v-if="scroll" class="modal-body modal-body-scroll">
            <div v-if="$slots.head" class="modal-pane-head">
              <slot name="head" />
            </div>
            <div class="modal-pane-scroll">
              <slot />
            </div>
            <div v-if="$slots.foot" class="modal-pane-foot">
              <slot name="foot" />
            </div>
          </div>
          <div v-else class="modal-body" :class="{ 'modal-body-fill': fill }">
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
