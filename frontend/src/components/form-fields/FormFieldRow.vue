<script setup lang="ts">
import { ref, computed } from "vue";
import { useI18n } from "vue-i18n";
import FormFieldRenderer from "./FormFieldRenderer.vue";
import FormulaComputeButton from "./FormulaComputeButton.vue";
import { useConfig } from "../../composables/useConfig";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { fieldLabel, fieldDescription } from "../../utils/pluginI18n";

// FormFieldRow - label + description (left/top) and the per-type
// renderer (right/bottom). When `field.collapsible === true` we add
// a ▶/▼ toggle in the label that hides the input cell, mirroring the
// original Formidable's `applyCollapsibleField` behaviour. Initial
// state defaults to `config.field_state_collapsed`.
//
// `i18nNamespace` opts the row into plugin-style field translation:
// when set (e.g. "plugin.test-plugin") and the field carries an
// `i18n: <base-key>` declaration, label/description resolve under
// `<namespace>.<base-key>.{label,description}` with literal fallback.
// Editor surfaces leave it unset so authors see the literal strings
// they're editing.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
  i18nNamespace?: string;
}>();

defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();
const { config } = useConfig();

const isCollapsible = computed(() => props.field.collapsible === true);
const collapsed = ref<boolean>(config.value?.field_state_collapsed === true);

const labelText = computed(() => fieldLabel(props.i18nNamespace, props.field));
const descriptionText = computed(() => fieldDescription(props.i18nNamespace, props.field));

// list and table values are top-level arrays (rows / items). Surface the
// count next to the label so authors see how full a collection is
// without expanding it. Null for every other type (no badge).
const ROW_COUNT_TYPES = new Set(["list", "table"]);
const rowCount = computed<number | null>(() => {
  if (!ROW_COUNT_TYPES.has(props.field.type)) return null;
  return Array.isArray(props.modelValue) ? props.modelValue.length : 0;
});
// table counts "rows" / "rijen"; list counts "items". The unit word is
// pluralized via vue-i18n's `singular | plural` form; the number is
// rendered separately so we don't depend on the plural-count variable.
const rowCountUnitKey = computed(() =>
  props.field.type === "table" ? "field.count.rows" : "field.count.items",
);

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
        {{ labelText }}
        <span v-if="rowCount !== null" class="form-field-count">{{ rowCount }} {{ t(rowCountUnitKey, rowCount) }}</span>
      </label>
      <p v-if="descriptionText" class="form-field-description">
        {{ descriptionText }}
      </p>
    </div>
    <div v-show="!(isCollapsible && collapsed)" class="form-field-input-cell">
      <FormFieldRenderer
        :field="field"
        :model-value="modelValue"
        @update:model-value="(v: unknown) => $emit('update:modelValue', v)"
      />
      <FormulaComputeButton :target-key="field.key" />
    </div>
  </div>
</template>
