<script setup lang="ts">
import { ref, computed } from "vue";
import { useI18n } from "vue-i18n";
import FormFieldRenderer from "./FormFieldRenderer.vue";
import { useConfig } from "../../composables/useConfig";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// FormFieldRow — label + description (left/top) and the per-type
// renderer (right/bottom). When `field.collapsible === true` we add
// a ▶/▼ toggle in the label that hides the input cell, mirroring the
// original Formidable's `applyCollapsibleField` behaviour. Initial
// state defaults to `config.field_state_collapsed`.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();
const { config } = useConfig();

const isCollapsible = computed(() => props.field.collapsible === true);
const collapsed = ref<boolean>(config.value?.field_state_collapsed === true);

function toggle() {
  collapsed.value = !collapsed.value;
}
</script>

<template>
  <div
    :class="[
      'form-field-row',
      { 'two-column': field.two_column, 'collapsible-field': isCollapsible, 'collapsed': isCollapsible && collapsed },
    ]"
  >
    <div class="form-field-label-cell">
      <label class="form-field-label">
        <button
          v-if="isCollapsible"
          type="button"
          class="collapse-toggle"
          :aria-expanded="!collapsed"
          :title="collapsed ? t('standard.expand') : t('standard.collapse')"
          @click="toggle"
        >{{ collapsed ? '▶' : '▼' }}</button>
        {{ field.label || field.key }}
      </label>
      <p v-if="field.description" class="form-field-description">
        {{ field.description }}
      </p>
    </div>
    <div v-show="!(isCollapsible && collapsed)" class="form-field-input-cell">
      <FormFieldRenderer
        :field="field"
        :model-value="modelValue"
        @update:model-value="(v: unknown) => $emit('update:modelValue', v)"
      />
    </div>
  </div>
</template>
