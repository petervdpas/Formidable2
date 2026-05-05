<script setup lang="ts">
import { computed } from "vue";
import TextField from "./TextField.vue";
import SelectField from "./SelectField.vue";

// Column shapes — one per field-type preset. The editor renders one
// row per option; each row has one cell per column. Storage is an
// array of plain objects keyed by column.key.

export type ColumnDef =
  | {
      key: string;
      type: "text";
      placeholder?: string;
    }
  | {
      key: string;
      type: "dropdown";
      options: string[];
      placeholder?: string;
      defaultValue?: string;
      /** Optional callback when this column changes — used by the
       *  "list" preset to lock value=[[custom]] on type=custom. */
      onChange?: (value: string, row: OptionRow, allRows: OptionRow[]) => OptionRow;
    };

export type OptionRow = Record<string, unknown>;

const props = defineProps<{
  columns: ColumnDef[];
}>();

const model = defineModel<OptionRow[]>({ default: () => [] });

function emptyRow(): OptionRow {
  const row: OptionRow = {};
  for (const c of props.columns) {
    if (c.type === "dropdown" && c.defaultValue !== undefined) {
      row[c.key] = c.defaultValue;
    } else {
      row[c.key] = "";
    }
  }
  return row;
}

function addRow() {
  model.value = [...model.value, emptyRow()];
}

function removeRow(idx: number) {
  model.value = model.value.filter((_, i) => i !== idx);
}

function setCell(idx: number, col: ColumnDef, value: string) {
  const next = model.value.map((r, i) => (i === idx ? { ...r, [col.key]: value } : r));
  if (col.type === "dropdown" && col.onChange) {
    next[idx] = col.onChange(value, next[idx], next);
  }
  model.value = next;
}

const visibleRows = computed(() => model.value);

function getCell(row: OptionRow, col: ColumnDef): string {
  const v = row[col.key];
  if (v == null) return "";
  return typeof v === "string" ? v : String(v);
}
</script>

<template>
  <div class="options-editor">
    <div class="options-rows">
      <div
        v-for="(row, i) in visibleRows"
        :key="i"
        class="options-row"
      >
        <template v-for="col in columns" :key="col.key">
          <TextField
            v-if="col.type === 'text'"
            :model-value="getCell(row, col)"
            @update:model-value="(v) => setCell(i, col, v)"
            :placeholder="col.placeholder"
            class="options-cell"
          />
          <SelectField
            v-else-if="col.type === 'dropdown'"
            :model-value="getCell(row, col)"
            @update:model-value="(v) => setCell(i, col, v)"
            :options="col.options.map((o) => ({ value: o, label: o }))"
            class="options-cell"
          />
        </template>
        <button
          type="button"
          class="btn-ghost-icon"
          @click="removeRow(i)"
          title="Remove option"
          aria-label="Remove option"
        >−</button>
      </div>
    </div>
    <button
      type="button"
      class="btn-ghost-block"
      @click="addRow"
      title="Add option"
      aria-label="Add option"
    >+</button>
  </div>
</template>
