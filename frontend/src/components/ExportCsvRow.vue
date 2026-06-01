<script setup lang="ts">
import { useI18n } from "vue-i18n";
import CsvTransformCell from "./CsvTransformCell.vue";
import { SwitchField } from "./fields";

export type ExportRow = {
  id: string;
  include: boolean;
  computed: boolean;
  header: string;
  sourceKeys: string[];
  separator: string;
  rule: string;
  param: string;
};

export type SourceOption = { value: string; label: string };

const props = defineProps<{
  row: ExportRow;
  sourceOptions: SourceOption[];
  labelByKey: Map<string, string>;
  preview: string;
}>();

const emit = defineEmits<{ (e: "remove"): void }>();

const { t } = useI18n();

function fieldLabel(key: string): string {
  return props.labelByKey.get(key) ?? key;
}

function addSource(fieldKey: string) {
  if (!fieldKey || props.row.sourceKeys.includes(fieldKey)) return;
  props.row.sourceKeys.push(fieldKey);
}

function removeSource(idx: number) {
  props.row.sourceKeys.splice(idx, 1);
}
</script>

<template>
  <tr>
    <td class="csv-export-td-narrow">
      <SwitchField v-model="row.include" />
    </td>
    <td>
      <span v-if="!row.computed" class="muted small">{{ fieldLabel(row.sourceKeys[0]) }}</span>
      <div v-else class="csv-export-chips">
        <span v-for="(key, ki) in row.sourceKeys" :key="ki" class="csv-export-chip">
          {{ fieldLabel(key) }}
          <button
            type="button"
            class="csv-export-chip-x"
            @click="removeSource(ki)"
            :aria-label="t('common.remove')"
          >×</button>
        </span>
        <select
          class="csv-export-chip-add"
          :value="''"
          @change="addSource(($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''"
        >
          <option value="">{{ t('csv.export.add.field') }}</option>
          <option v-for="o in sourceOptions" :key="o.value" :value="o.value">
            {{ o.label }}
          </option>
        </select>
      </div>
    </td>
    <td>
      <input v-model="row.header" class="csv-export-header-input" />
    </td>
    <td class="csv-export-td-narrow">
      <span v-if="!row.computed" class="muted">-</span>
      <input v-else v-model="row.separator" class="csv-import-concat-input" />
    </td>
    <td>
      <CsvTransformCell v-model:rule="row.rule" v-model:param="row.param" />
    </td>
    <td class="csv-import-td-preview muted small">
      {{ row.include ? preview : "" }}
    </td>
    <td class="csv-export-td-narrow">
      <button
        v-if="row.computed"
        type="button"
        class="csv-export-row-x"
        :aria-label="t('common.remove')"
        @click="emit('remove')"
      >×</button>
    </td>
  </tr>
</template>
