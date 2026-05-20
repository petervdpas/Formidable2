<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import Popup from "./Popup.vue";
import FacetIcon from "./FacetIcon.vue";
import type { Facet } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  facet: Facet;
  /** "" = no filter (all forms); otherwise the option LABEL to keep
   *  (matches set=true && selected==label). */
  modelValue: string;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", v: string): void;
}>();

const { t } = useI18n();

const active = computed(() =>
  props.facet.options.find((o) => o.label === props.modelValue),
);

const triggerColorClass = computed(() => {
  if (active.value) return `expr-text-${active.value.color}`;
  return "facet-picker-empty";
});

const triggerLabel = computed(() =>
  active.value ? active.value.label : props.facet.key,
);

function pick(label: string, close: () => void) {
  if (label !== props.modelValue) emit("update:modelValue", label);
  close();
}
</script>

<template>
  <Popup placement="below">
    <template #trigger="{ toggle, open }">
      <button
        type="button"
        class="facet-filter-trigger"
        :class="{ open, 'is-active': !!active }"
        @click="toggle"
      >
        <FacetIcon :icon="facet.icon" :class="triggerColorClass" />
        <span class="facet-filter-label">{{ triggerLabel }}</span>
      </button>
    </template>

    <template #default="{ close }">
      <div class="facet-picker-panel" role="menu">
        <button
          type="button"
          class="facet-picker-row"
          :class="{ active: !active }"
          role="menuitem"
          @click="pick('', close)"
        >
          <span class="facet-picker-swatch facet-picker-swatch--empty"></span>
          <span class="facet-picker-label">{{ t('workspace.storage.facet_filter.none') }}</span>
        </button>

        <button
          v-for="o in facet.options"
          :key="o.label"
          type="button"
          class="facet-picker-row"
          :class="{ active: o.label === modelValue }"
          role="menuitem"
          @click="pick(o.label, close)"
        >
          <span class="facet-picker-swatch" :class="`facet-swatch-${o.color}`"></span>
          <span class="facet-picker-label">{{ o.label }}</span>
        </button>
      </div>
    </template>
  </Popup>
</template>
