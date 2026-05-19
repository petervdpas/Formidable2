<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import Popup from "./Popup.vue";
import type { Facet } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { FacetState } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";

const props = withDefaults(
  defineProps<{
    /** The facet whose dimension this picker represents. */
    facet: Facet;
    /** Current per-record state for this facet (set + selected label). */
    modelValue: FacetState;
    /** When true the icon still renders but the picker does not open. */
    disabled?: boolean;
    /** Visual size hint. "sm" for inline rows, "md" for the meta block. */
    size?: "sm" | "md";
    /** Forwarded to the underlying Popup so the meta-block corner
     *  variant can open down-left instead of clipping under the
     *  slide-out tabs. */
    placement?: "below" | "below-left";
  }>(),
  { disabled: false, size: "md", placement: "below" },
);

const emit = defineEmits<{
  (e: "update:modelValue", v: FacetState): void;
}>();

const { t } = useI18n();

const activeOption = computed(() =>
  props.facet.options.find((o) => o.label === props.modelValue.selected),
);

// Icon color: chosen option's color when set+selected; muted gray when
// set but no label picked; full muted when unset entirely.
const iconColorClass = computed(() => {
  if (activeOption.value) return `expr-text-${activeOption.value.color}`;
  if (props.modelValue.set) return "expr-text-gray";
  return "facet-picker-empty";
});

const iconClass = computed(() => `fa-solid ${props.facet.icon}`);

function pick(label: string, close: () => void) {
  if (label !== props.modelValue.selected || !props.modelValue.set) {
    emit(
      "update:modelValue",
      new FacetState({ set: true, selected: label }),
    );
  }
  close();
}
function clear(close: () => void) {
  if (props.modelValue.set || props.modelValue.selected !== "") {
    emit("update:modelValue", new FacetState({ set: false, selected: "" }));
  }
  close();
}
</script>

<template>
  <Popup :placement="placement">
    <template #trigger="{ toggle, open }">
      <button
        type="button"
        class="facet-picker-trigger"
        :class="[
          `facet-picker-trigger--${size}`,
          { open, 'is-empty': !modelValue.set && !activeOption },
        ]"
        :disabled="disabled"
        :title="activeOption ? activeOption.label : facet.key"
        :aria-label="activeOption ? activeOption.label : facet.key"
        @click="toggle"
      >
        <i :class="[iconClass, iconColorClass]" aria-hidden="true"></i>
      </button>
    </template>

    <template #default="{ close }">
      <div class="facet-picker-panel" role="menu">
        <p v-if="facet.options.length === 0" class="facet-picker-empty-msg">
          {{ t('facet.picker.no_options') }}
        </p>

        <button
          v-for="o in facet.options"
          :key="o.label"
          type="button"
          class="facet-picker-row"
          :class="{ active: o.label === modelValue.selected && modelValue.set }"
          role="menuitem"
          @click="pick(o.label, close)"
        >
          <span class="facet-picker-swatch" :class="`facet-swatch-${o.color}`"></span>
          <span class="facet-picker-label">{{ o.label }}</span>
        </button>

        <button
          type="button"
          class="facet-picker-row facet-picker-clear"
          :class="{ active: !modelValue.set && !activeOption }"
          role="menuitem"
          @click="clear(close)"
        >
          <span class="facet-picker-swatch facet-picker-swatch--empty"></span>
          <span class="facet-picker-label">{{ t('facet.picker.clear') }}</span>
        </button>
      </div>
    </template>
  </Popup>
</template>
