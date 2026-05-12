<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { Clipboard } from "@wailsio/runtime";
import { useToast } from "../composables/useToast";

// CopyButton — single-source clipboard action. Routes through Wails'
// Clipboard runtime instead of navigator.clipboard because webviews
// block the browser API outside secure contexts.
//
// `text` accepts three shapes so the same component covers static
// strings, cheap sync getters, and async sources that need a backend
// call to produce the payload (e.g. RenderFullHTML). The function
// form is invoked ONLY on click — never during render — so a
// side-effecting getter is safe here.
//
// Auto-disable is intentionally not derived from `text`; callers
// pass `:disabled="..."` explicitly so the disable rule stays
// readable at the call site (and async sources don't need a
// separate "is the underlying source ready" computed).
type TextSource = string | (() => string | Promise<string>);

const props = withDefaults(defineProps<{
  text: TextSource;
  /** i18n key for tooltip / aria-label. Defaults to common.copy. */
  titleKey?: string;
  /** i18n key for the success toast. Defaults to common.copied. */
  successKey?: string;
  /** i18n key for the failure toast. Defaults to common.copy_error. */
  errorKey?: string;
  disabled?: boolean;
  /** Hide the FA icon (text-only button). */
  iconOnly?: boolean;
  /** Optional extra class on the button (e.g. "right-slideout-action"). */
  buttonClass?: string;
}>(), {
  titleKey: "common.copy",
  successKey: "common.copied",
  errorKey: "common.copy_error",
  disabled: false,
  iconOnly: true,
  buttonClass: "tool-btn",
});

const { t } = useI18n();
const toast = useToast();

async function copy() {
  if (props.disabled) return;
  let value: string;
  try {
    if (typeof props.text === "string") {
      value = props.text;
    } else {
      const out = props.text();
      value = out instanceof Promise ? await out : out;
    }
  } catch {
    toast.error(props.errorKey);
    return;
  }
  if (!value) return;
  try {
    await Clipboard.SetText(value);
    toast.success(props.successKey);
  } catch {
    toast.error(props.errorKey);
  }
}
</script>

<template>
  <button
    type="button"
    :class="buttonClass"
    :disabled="disabled"
    :title="t(titleKey)"
    :aria-label="t(titleKey)"
    @click="copy"
  >
    <i v-if="iconOnly" class="fa-solid fa-copy" aria-hidden="true"></i>
    <span v-else>{{ t(titleKey) }}</span>
  </button>
</template>
