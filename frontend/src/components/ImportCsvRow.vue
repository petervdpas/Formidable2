<script setup lang="ts">
import CsvTransformCell from "./CsvTransformCell.vue";
import { useI18n } from "vue-i18n";

export type Mapping = {
  header: string;
  fieldKey: string;
  rule: string;
  param: string;
};

export type TargetOption = { value: string; label: string };

defineProps<{
  mapping: Mapping;
  targetOptions: TargetOption[];
  rawPreview: string;
  typedPreview: string;
}>();

const { t } = useI18n();
</script>

<template>
  <tr>
    <td class="csv-import-td-header">{{ mapping.header }}</td>
    <td>
      <select v-model="mapping.fieldKey">
        <option value="">{{ t('csv.skip') }}</option>
        <option v-for="o in targetOptions" :key="o.value" :value="o.value">
          {{ o.label }}
        </option>
      </select>
    </td>
    <td>
      <CsvTransformCell v-model:rule="mapping.rule" v-model:param="mapping.param" />
    </td>
    <td class="csv-import-td-preview muted small">{{ rawPreview }}</td>
    <td class="csv-import-td-preview">{{ typedPreview }}</td>
  </tr>
</template>
