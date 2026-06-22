<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { FormSwitchRow } from "./fields";

// Owns how a template's storage record list is ordered. Today it's a single
// "sort by item field" toggle (off = filename order); kept as its own component
// so it can grow (asc/desc, sort-field picker) without bloating the template
// setup section. Presentational: the parent owns the draft.
const model = defineModel<boolean>({ default: false });

const { t } = useI18n();
</script>

<template>
  <FormSwitchRow
    v-model="model"
    :label="t('workspace.templates.setup.sort_by_item_field')"
    :description="t('workspace.templates.setup.sort_by_item_field_hint')"
  />
</template>
