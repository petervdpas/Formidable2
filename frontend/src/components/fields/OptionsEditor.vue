<script setup lang="ts">
import { computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import TextField from "./TextField.vue";
import SelectField from "./SelectField.vue";
import DateInput from "./DateInput.vue";
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
      type: "color";
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
  /** Overrides how this row's editable (label) cell renders: "format" /
   *  "timeblock" (a dropdown of `choices`), "color" (a picker), "number",
   *  "date" (a date picker), else text. */
  input?: string;
  choices?: string[];
};

const props = defineProps<{
  columns: ColumnDef[];
  fixedRows?: FixedRowConfig[];
  /** Column keys rendered read-only across every row - e.g. the
   *  structural "value" of a boolean / range fixed shape, where only
   *  the label is editable. */
  lockedColumns?: string[];
  /** Allow free-form rows AFTER the fixed rows: the fixed rows stay locked,
   *  and the user can add/remove extra value/label rows below them. Used by
   *  `project` (fixed axis rows + author-added lanes). */
  allowExtraRows?: boolean;
  /** Translated label for the extra rows + add button (project: "Lane"), so the
   *  free-form section isn't a mystery "+". */
  extraRowsLabel?: string;
}>();

const fixedCount = computed(() => props.fixedRows?.length ?? 0);
// A row is "fixed" when its index falls within the fixed-rows shape; rows beyond
// it are free-form extras (lanes).
function isFixedRow(i: number): boolean {
  return i < fixedCount.value;
}

// Locked columns apply only to fixed rows; extra rows are fully editable (a
// lane's value is its author-chosen id).
function isLocked(key: string, i: number): boolean {
  return isFixedRow(i) && !!props.lockedColumns?.includes(key);
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
    const fixedAligned = props.fixedRows.map((fr, i) => {
      const existing = model.value[i];
      return existing ? { ...fr.defaults, ...existing } : { ...fr.defaults };
    });
    // Preserve any extra (lane) rows the user added below the fixed rows.
    const extras = props.allowExtraRows ? model.value.slice(total) : [];
    const aligned = [...fixedAligned, ...extras];
    // Skip the assignment when the shape already matches to avoid an
    // infinite watcher loop. Extras are the same object refs, so only the
    // fixed portion and the total length can differ.
    const same =
      model.value.length === aligned.length &&
      fixedAligned.every(
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
    <draggable
      :model-value="visibleRows"
      tag="div"
      class="options-rows"
      handle=".dnd-handle"
      :disabled="isFixed"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      :item-key="(_e: OptionRow, i: number) => i"
      @update:model-value="(rows: OptionRow[]) => (model = rows)"
    >
      <template #item="{ element: row, index: i }">
      <div class="options-row-group">
        <div class="options-row">
          <span
            v-if="!isFixed"
            class="dnd-handle"
            aria-hidden="true"
          >⠿</span>
          <span
            v-if="isFixedRow(i) && fixedRows && fixedRows[i]"
            class="options-row-label small"
          >{{ t(fixedRows[i].labelKey) }}</span>
          <span
            v-else-if="!isFixedRow(i) && extraRowsLabel"
            class="options-row-label small"
          >{{ extraRowsLabel }}</span>
          <template v-for="col in columns" :key="col.key">
            <!-- Fixed shapes hide the locked structural cell (its snake_case value
                 key is redundant with the gutter label), leaving label + control. -->
            <template v-if="isLocked(col.key, i)"></template>
            <!-- Per-row input override (fixed shapes): the editable "label" cell
                 can be a colour picker / number / preset dropdown per row. -->
            <template v-else-if="isFixedRow(i) && fixedRows && fixedRows[i] && fixedRows[i].input && col.key === 'label'">
              <input
                v-if="fixedRows[i].input === 'color'"
                type="color" class="options-cell options-color"
                :value="getCell(row, col) || '#000000'"
                @input="setCell(i, col, ($event.target as HTMLInputElement).value)"
              />
              <input
                v-else-if="fixedRows[i].input === 'number'"
                type="number" min="0" class="options-cell"
                :value="getCell(row, col)"
                @input="setCell(i, col, ($event.target as HTMLInputElement).value)"
              />
              <DateInput
                v-else-if="fixedRows[i].input === 'date'"
                :model-value="getCell(row, col)"
                @update:model-value="(v) => setCell(i, col, v)"
                class="options-cell"
              />
              <SelectField
                v-else-if="fixedRows[i].input === 'format' || fixedRows[i].input === 'timeblock'"
                :model-value="getCell(row, col)"
                @update:model-value="(v) => setCell(i, col, v)"
                :options="(fixedRows[i].choices ?? []).map((o) => ({ value: o, label: o }))"
                class="options-cell"
              />
              <TextField
                v-else
                :model-value="getCell(row, col)"
                @update:model-value="(v) => setCell(i, col, v)"
                :placeholder="col.placeholder"
                class="options-cell"
              />
            </template>
            <TextField
              v-else-if="col.type === 'text'"
              :model-value="getCell(row, col)"
              @update:model-value="(v) => setCell(i, col, v)"
              :placeholder="col.placeholder"
              :readonly="isLocked(col.key, i)"
              class="options-cell"
            />
            <input
              v-else-if="col.type === 'color'"
              type="color"
              class="options-cell options-color"
              :value="getCell(row, col) || '#5361c9'"
              @input="setCell(i, col, ($event.target as HTMLInputElement).value)"
            />
            <SelectField
              v-else-if="col.type === 'dropdown'"
              :model-value="getCell(row, col)"
              @update:model-value="(v) => setCell(i, col, v)"
              :options="col.options.map((o) => ({ value: o, label: o }))"
              :disabled="isLocked(col.key, i)"
              class="options-cell"
            />
          </template>
          <button
            v-if="!isFixedRow(i)"
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
      </template>
    </draggable>
    <button
      v-if="!isFixed || allowExtraRows"
      type="button"
      class="btn-ghost-block"
      @click="addRow"
      :title="extraRowsLabel ? `+ ${extraRowsLabel}` : 'Add option'"
      :aria-label="extraRowsLabel ? `Add ${extraRowsLabel}` : 'Add option'"
    >{{ extraRowsLabel ? `+ ${extraRowsLabel}` : "+" }}</button>
  </div>
</template>
