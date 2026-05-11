<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import Popup from "./Popup.vue";
import type { FlagDefinition } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = withDefaults(
  defineProps<{
    /** Available states for this template; empty = no picker (only clear/none). */
    definitions: FlagDefinition[];
    /** Selected LABEL, or "" for "no state". */
    modelValue: string;
    /** When true, the picker still renders the icon but does not open. */
    disabled?: boolean;
    /** Render a generic uncolored flag when value is "" but the form
     *  has a legacy `flagged: true` (so users can still clear it). */
    legacyFlagged?: boolean;
    /** Visual size hint. "sm" for inline rows, "md" for the meta block. */
    size?: "sm" | "md";
  }>(),
  { disabled: false, legacyFlagged: false, size: "md" },
);

const emit = defineEmits<{
  (e: "update:modelValue", v: string): void;
}>();

const { t } = useI18n();

const active = computed(() => props.definitions.find((d) => d.label === props.modelValue));

// Icon color: the current state's color when set; a neutral gray when
// nothing is set (with a slightly different shade if the legacy bool
// is on, so users see "marked but no state").
const iconColorClass = computed(() => {
  if (active.value) return `expr-text-${active.value.color}`;
  if (props.legacyFlagged) return "expr-text-gray";
  return "flag-picker-empty";
});

function pick(label: string, close: () => void) {
  if (label !== props.modelValue) emit("update:modelValue", label);
  close();
}
function clear(close: () => void) {
  if (props.modelValue !== "") emit("update:modelValue", "");
  close();
}
</script>

<template>
  <Popup placement="below">
    <template #trigger="{ toggle, open }">
      <button
        type="button"
        class="flag-picker-trigger"
        :class="[`flag-picker-trigger--${size}`, { open, 'is-empty': !active && !legacyFlagged }]"
        :disabled="disabled"
        :title="active ? active.label : t('flag.picker.title_empty')"
        :aria-label="active ? active.label : t('flag.picker.title_empty')"
        @click="toggle"
      >
        <i class="fa-solid fa-flag" :class="iconColorClass" aria-hidden="true"></i>
      </button>
    </template>

    <template #default="{ close }">
      <div class="flag-picker-panel" role="menu">
        <p v-if="definitions.length === 0" class="flag-picker-empty-msg">
          {{ t('flag.picker.no_definitions') }}
        </p>

        <button
          v-for="d in definitions"
          :key="d.label"
          type="button"
          class="flag-picker-row"
          :class="{ active: d.label === modelValue }"
          role="menuitem"
          @click="pick(d.label, close)"
        >
          <span class="flag-picker-swatch" :class="`expr-bg-${d.color}`"></span>
          <span class="flag-picker-label">{{ d.label }}</span>
        </button>

        <button
          type="button"
          class="flag-picker-row flag-picker-clear"
          :class="{ active: !active }"
          role="menuitem"
          @click="clear(close)"
        >
          <span class="flag-picker-swatch flag-picker-swatch--empty"></span>
          <span class="flag-picker-label">{{ t('flag.picker.clear') }}</span>
        </button>
      </div>
    </template>
  </Popup>
</template>
