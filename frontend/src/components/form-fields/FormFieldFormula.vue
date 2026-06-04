<script setup lang="ts">
/*
 * FormFieldFormula - virtual field renderer (sibling to FormFieldFacet).
 *
 * A formula field carries no value in storage Form.data. The backend
 * writes the bound formula's output into the target data field on load
 * or on save (storage.applyFormulaFields). This renderer is a read-only
 * projection of that target's current value, so the author sees the
 * computed result inline; editing happens in the target field's own row.
 *
 * Wiring: the parent workspace provides `formValues` (the live draft
 * values map). This component injects it and reads values[target_key].
 * When inject is absent (e.g. a plugin form rendering FormFieldRow in
 * isolation) the field shows a small "not available" hint.
 */
import { computed, inject } from "vue";
import { useI18n } from "vue-i18n";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { FORM_VALUES_KEY } from "../../composables/formValues";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const { t } = useI18n();

const values = inject(FORM_VALUES_KEY, null);

const targetKey = computed(() => props.field.target_key ?? "");

const targetValue = computed<string>(() => {
  if (!values || !targetKey.value) return "";
  const v = values.value[targetKey.value];
  if (v == null) return "";
  if (Array.isArray(v)) return v.join(", ");
  return String(v);
});

const triggerLabel = computed(() =>
  props.field.trigger === "load"
    ? t("workspace.templates.field_edit.formula.trigger_load")
    : t("workspace.templates.field_edit.formula.trigger_save"),
);

const unavailableMessage = computed(() => {
  if (!values) return t("formula.field.unavailable");
  if (!targetKey.value) return t("formula.field.missing_binding");
  return "";
});
</script>

<template>
  <div v-if="unavailableMessage" class="formula-field-unavailable muted small">
    {{ unavailableMessage }}
  </div>
  <div v-else class="formula-field">
    <output class="formula-field-value" :class="{ 'is-empty': !targetValue }">
      {{ targetValue || t("formula.field.no_value") }}
    </output>
    <span class="formula-field-hint muted small">
      {{ t("formula.field.writes", { formula: field.formula_key, target: targetKey, trigger: triggerLabel }) }}
    </span>
  </div>
</template>
