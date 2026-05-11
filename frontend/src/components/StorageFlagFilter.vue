<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import Popup from "./Popup.vue";
import type { FlagDefinition } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  definitions: FlagDefinition[];
  /** "" = no filter (all forms); otherwise the state LABEL to keep. */
  modelValue: string;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", v: string): void;
}>();

const { t } = useI18n();

const active = computed(() => props.definitions.find((d) => d.label === props.modelValue));

const triggerColorClass = computed(() => {
  if (active.value) return `expr-text-${active.value.color}`;
  return "flag-picker-empty";
});

const triggerLabel = computed(() => {
  if (active.value) return active.value.label;
  return t("workspace.storage.flag_filter.all");
});

function pick(state: string, close: () => void) {
  if (state !== props.modelValue) emit("update:modelValue", state);
  close();
}
</script>

<template>
  <Popup placement="below">
    <template #trigger="{ toggle, open }">
      <button
        type="button"
        class="flag-filter-trigger"
        :class="{ open, 'is-active': !!active }"
        @click="toggle"
      >
        <i class="fa-solid fa-flag" :class="triggerColorClass" aria-hidden="true"></i>
        <span class="flag-filter-label">{{ triggerLabel }}</span>
      </button>
    </template>

    <template #default="{ close }">
      <div class="flag-picker-panel" role="menu">
        <button
          type="button"
          class="flag-picker-row"
          :class="{ active: !active }"
          role="menuitem"
          @click="pick('', close)"
        >
          <span class="flag-picker-swatch flag-picker-swatch--empty"></span>
          <span class="flag-picker-label">{{ t('workspace.storage.flag_filter.all') }}</span>
        </button>

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
      </div>
    </template>
  </Popup>
</template>
