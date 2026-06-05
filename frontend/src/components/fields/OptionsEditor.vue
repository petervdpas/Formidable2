<script setup lang="ts">
import { computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import TextField from "./TextField.vue";
import SelectField from "./SelectField.vue";
import OptionsSubRow from "./OptionsSubRow.vue";

const { t } = useI18n();

// Column shapes - one per field-type preset. The editor renders one
// row per option; each row has one cell per column. Storage is an
// array of plain objects keyed by column.key.

// SubRowConfig fires when a dropdown column's value matches a key in
// `perValue`. It surfaces an extra structured widget below the main
// row, bound to row[rowKey]. The stored value is always a
// pipe-delimited "value:label|value:label" string - the form-side
// parseChoices() handles the splitting.
//
// All user-facing strings on these descriptors are i18n keys
// (resolved via vue-i18n in OptionsSubRow). Backend owns the keys;
// the locale catalog owns the wording.
export type SubRowVariant = {
  placeholderKey?: string;
  labelKey?: string;
  /** Where this variant stores its value on the row. Falls back to the
   *  parent SubRowConfig.rowKey when unset. Lets distinct column types
   *  under one dropdown write to different keys (choices vs step). */
  rowKey?: string;
  /** Scalar mode: a single raw value stored verbatim at rowKey (no
   *  value:label pair serialization). Used by a number column's step. */
  scalar?: boolean;
  /** Placeholder/fallback shown when a scalar cell is empty. */
  defaultValue?: string;
  /** When set, the editor clamps the pair count to this many. */
  maxEntries?: number;
  /** When set, the widget renders one fixed row per entry. Each row
   *  locks its `value` (e.g. "true"/"false" for a bool column) and
   *  the user only edits the label. `labelKey` is the i18n key for
   *  the gutter caption beside the locked value. Mutually exclusive
   *  with the free-form add/remove mode. */
  entries?: { labelKey: string; value: string; placeholderKey?: string }[];
};

export type SubRowConfig = {
  rowKey: string;
  perValue: Record<string, SubRowVariant>;
};

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
      /** Optional callback when this column changes - used by the
       *  "list" preset to lock value=[[custom]] on type=custom. */
      onChange?: (value: string, row: OptionRow, allRows: OptionRow[]) => OptionRow;
      /** Optional sub-row that appears under the main row when this
       *  column's value matches a key in subRow.perValue. */
      subRow?: SubRowConfig;
    };

export type OptionRow = Record<string, unknown>;

// When set, the editor renders exactly fixedRows.length rows - no
// add/remove buttons, no row-count drift. Each row carries an i18n
// key for its gutter caption and a defaults object that fills
// missing cells when the model arrives shorter than the fixed arity.
export type FixedRowConfig = {
  labelKey: string;
  defaults: OptionRow;
};

const props = defineProps<{
  columns: ColumnDef[];
  fixedRows?: FixedRowConfig[];
  /** Column keys rendered read-only across every row - e.g. the
   *  structural "value" of a boolean / range fixed shape, where only
   *  the label is editable. */
  lockedColumns?: string[];
}>();

function isLocked(key: string): boolean {
  return !!props.lockedColumns?.includes(key);
}

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

// Keep the model aligned with fixedRows on every change. Preserves
// user-supplied cells where present, fills defaults otherwise, and
// truncates excess rows that would have come from a previous
// free-form field type (so switching `type=text → bool` doesn't
// leave a 7-entry options array).
const isFixed = computed(() => !!props.fixedRows && props.fixedRows.length > 0);

watch(
  [() => props.fixedRows, model],
  () => {
    if (!props.fixedRows) return;
    const total = props.fixedRows.length;
    const aligned = props.fixedRows.map((fr, i) => {
      const existing = model.value[i];
      return existing ? { ...fr.defaults, ...existing } : { ...fr.defaults };
    });
    // Skip the assignment when the shape already matches to avoid an
    // infinite watcher loop.
    const same =
      model.value.length === total &&
      aligned.every(
        (r, i) =>
          model.value[i] &&
          Object.keys(r).every((k) => model.value[i][k] === r[k]),
      );
    if (!same) model.value = aligned;
  },
  { immediate: true, deep: true },
);

function setCell(idx: number, col: ColumnDef, value: string) {
  const next = model.value.map((r, i) => (i === idx ? { ...r, [col.key]: value } : r));
  if (col.type === "dropdown" && col.onChange) {
    next[idx] = col.onChange(value, next[idx], next);
  }
  model.value = next;
}

// Sub-row resolution: for each dropdown column with a subRow config,
// look up the variant matching the row's current value of that column.
// Returns null when no variant applies (no extra row rendered). The
// actual editing is delegated to <OptionsSubRow>; this editor only
// owns wiring (which row in the model gets the value).
type ActiveSubRow = {
  rowKey: string;
  variant: SubRowVariant;
};

function activeSubRow(row: OptionRow, col: ColumnDef): ActiveSubRow | null {
  if (col.type !== "dropdown" || !col.subRow) return null;
  const current = getCell(row, col);
  const variant = col.subRow.perValue[current];
  if (!variant) return null;
  return { rowKey: variant.rowKey ?? col.subRow.rowKey, variant };
}

function subValue(row: OptionRow, rowKey: string): string {
  return String(row[rowKey] ?? "");
}

function writeSubValue(rowIdx: number, rowKey: string, value: string): void {
  model.value = model.value.map((r, i) =>
    i === rowIdx ? { ...r, [rowKey]: value } : r,
  );
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
        class="options-row-group"
      >
        <div class="options-row">
          <span
            v-if="isFixed && fixedRows && fixedRows[i]"
            class="options-row-label small"
          >{{ t(fixedRows[i].labelKey) }}</span>
          <template v-for="col in columns" :key="col.key">
            <TextField
              v-if="col.type === 'text'"
              :model-value="getCell(row, col)"
              @update:model-value="(v) => setCell(i, col, v)"
              :placeholder="col.placeholder"
              :readonly="isLocked(col.key)"
              class="options-cell"
            />
            <SelectField
              v-else-if="col.type === 'dropdown'"
              :model-value="getCell(row, col)"
              @update:model-value="(v) => setCell(i, col, v)"
              :options="col.options.map((o) => ({ value: o, label: o }))"
              :disabled="isLocked(col.key)"
              class="options-cell"
            />
          </template>
          <button
            v-if="!isFixed"
            type="button"
            class="btn-ghost-icon"
            @click="removeRow(i)"
            title="Remove option"
            aria-label="Remove option"
          >−</button>
        </div>
        <template v-for="col in columns" :key="col.key + '-sub'">
          <OptionsSubRow
            v-if="activeSubRow(row, col)"
            :variant="activeSubRow(row, col)!.variant"
            :model-value="subValue(row, activeSubRow(row, col)!.rowKey)"
            @update:model-value="(v) => writeSubValue(i, activeSubRow(row, col)!.rowKey, v)"
          />
        </template>
      </div>
    </div>
    <button
      v-if="!isFixed"
      type="button"
      class="btn-ghost-block"
      @click="addRow"
      title="Add option"
      aria-label="Add option"
    >+</button>
  </div>
</template>
