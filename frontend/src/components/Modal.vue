<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch, nextTick, useTemplateRef } from "vue";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    /** Close when the backdrop beside the dialog is clicked. Off by
     *  default: a stray click (or a text-selection drag that ends outside
     *  the dialog) shouldn't discard an open form. Esc and the × button
     *  still close. Opt in per-modal where backdrop-dismiss is wanted. */
    closeOnBackdrop?: boolean;
    closeOnEsc?: boolean;
    /** Width override for the dialog box (CSS value, e.g. "480px"). */
    width?: string;
    /** Optional class added to the dialog (e.g. for type-tinting). */
    dialogClass?: string;
    /** Optional inline style merged into the dialog (e.g. CSS vars). */
    dialogStyle?: Record<string, string>;
    /** Render an expand/restore button in the header. Accepts:
     *  - true / "full": expand to the full viewport (minus backdrop padding).
     *  - a number: expand to that many px wide, height stays content-driven
     *    (capped by the 90vh max-height). Use for dialogs that just need
     *    more room, not a full takeover.
     *  - false (default): no expand button. */
    maximizable?: boolean | "full" | number;
    /** When true, the dialog body is a flex column: the #head and #foot
     *  slots stay pinned and the default slot scrolls between them. Use
     *  for long forms/tables so the controls and column headers don't
     *  scroll out of view. */
    scroll?: boolean;
    /** When true, the body is a flex column whose single child stretches
     *  to fill the dialog height (so content grows when maximized). Give
     *  the dialog a height via dialogStyle for the non-maximized case. */
    fill?: boolean;
    /** When true, raise the backdrop above popups (popup.css 1100-1200).
     *  Use for a dialog launched from inside a Popup so it renders over it. */
    elevated?: boolean;
  }>(),
  {
    closeOnBackdrop: false,
    closeOnEsc: true,
    width: "480px",
    maximizable: false,
    scroll: false,
    fill: false,
    elevated: false,
  },
);

const emit = defineEmits<{ (e: "close"): void }>();

const maximized = ref(false);

const canMaximize = computed(
  () =>
    props.maximizable === true ||
    props.maximizable === "full" ||
    typeof props.maximizable === "number",
);

// When maximized, keep the caller's dialogStyle (it may carry CSS custom
// properties the dialog class depends on, e.g. the field editor's --type-bg
// tint); only width/height are overridden. Restoring snaps back - no animation.
const computedStyle = computed(() => {
  const base = { width: props.width, ...(props.dialogStyle || {}) };
  if (!maximized.value) return base;
  // A numeric maximizable grows the dialog to that px width and leaves the
  // height content-driven (max-height 90vh still caps it); true / "full" takes
  // over the full viewport.
  if (typeof props.maximizable === "number") {
    return { ...base, width: `${props.maximizable}px`, height: "auto" };
  }
  return {
    ...base,
    width: "calc(100vw - var(--space-4) * 2)",
    height: "calc(100vh - var(--space-4) * 2)",
  };
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
      <div v-if="open" class="modal-backdrop" :class="{ 'modal-elevated': elevated }" @click.self="onBackdropClick">
        <div
          ref="dialog"
          :class="['modal-dialog', dialogClass, { 'modal-scrolling': scroll, 'modal-maximized': maximized }]"
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
              v-if="canMaximize"
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
