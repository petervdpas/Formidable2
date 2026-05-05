<script setup lang="ts">
import { computed } from "vue";
import draggable from "vuedraggable";
import { TextField, SelectField, SwitchField, type SelectOption } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// DnD scope — unique per component instance so drags don't cross
// table fields when multiple are rendered (e.g. inside loop entries).
const dndScope =
  "table:" +
  (typeof crypto !== "undefined" && crypto.randomUUID
    ? crypto.randomUUID()
    : Math.random().toString(36).slice(2)
  ).slice(0, 8);

// FormFieldTable mirrors the original Formidable's `table` renderer
// (utils/fieldFactory.js). Storage shape is Array<Array<any>> — one
// row is one array of cells in column order.
//
// Per-column type comes from field.options[i].type:
//   "string" (default), "number", "date", "bool", "dropdown"
// "reference" + drag-reorder + paste-data are deferred to a follow-up.

type Col = {
  key: string;
  label: string;
  type: "string" | "number" | "date" | "bool" | "dropdown";
  choices: SelectOption[];
};

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

// ── Columns ──────────────────────────────────────────────────────────
// Each option may be {value, label, type, choices}. `choices` is a
// pipe-separated "key:Label | key:Label" string in the original
// schema; we parse it once into SelectOption[] for the SelectField.
const columns = computed<Col[]>(() => {
  const raw = props.field.options ?? [];
  return raw
    .filter((o): o is Record<string, unknown> => !!o && typeof o === "object")
    .map((o) => {
      const type = String(o.type ?? "string");
      const valid = ["string", "number", "date", "bool", "dropdown"];
      return {
        key: String(o.value ?? ""),
        label: String(o.label ?? o.value ?? ""),
        type: (valid.includes(type) ? type : "string") as Col["type"],
        choices: parseChoices(String(o.choices ?? "")),
      };
    });
});

// "key:Label | key:Label" → [{value:"key", label:"Label"}]
function parseChoices(s: string): SelectOption[] {
  if (!s) return [];
  return s
    .split("|")
    .map((piece) => piece.trim())
    .filter(Boolean)
    .map((piece) => {
      const [v, l] = piece.split(":").map((p) => p.trim());
      return { value: v ?? "", label: l ?? v ?? "" };
    });
}

// ── Rows ─────────────────────────────────────────────────────────────
// Coerce stored value into a 2D array, padding short rows so every
// row has one cell per column. Cells are kept verbatim (preserves
// type round-trip when columns change shape later).
const rows = computed<unknown[][]>({
  get: () => {
    const v = props.modelValue;
    if (!Array.isArray(v)) return [];
    return v.map((r) => {
      if (Array.isArray(r)) return r.slice();
      return [];
    });
  },
  set: (v) => emit("update:modelValue", v),
});

function emitRows(next: unknown[][]) {
  emit("update:modelValue", next);
}

function setCell(rowIdx: number, colIdx: number, val: unknown) {
  const next = rows.value.map((r) => r.slice());
  while (next[rowIdx].length < columns.value.length) next[rowIdx].push("");
  next[rowIdx][colIdx] = val;
  emitRows(next);
}

function addRow() {
  const empty = columns.value.map((c) => emptyCell(c.type));
  emitRows([...rows.value, empty]);
}

function removeRow(idx: number) {
  emitRows(rows.value.filter((_, i) => i !== idx));
}

function emptyCell(type: Col["type"]): unknown {
  if (type === "bool") return false;
  if (type === "number") return 0;
  return "";
}

// ── Cell coercion helpers — bridge between stored shape and UI inputs.
function asString(v: unknown): string {
  if (v == null) return "";
  if (typeof v === "string") return v;
  return String(v);
}

function asBool(v: unknown): boolean {
  if (typeof v === "boolean") return v;
  if (typeof v === "string") return v.toLowerCase() === "true";
  if (typeof v === "number") return v !== 0;
  return false;
}

function asNumber(v: unknown): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}
</script>

<template>
  <div class="table-field">
    <table v-if="columns.length > 0" class="ff-table">
      <thead>
        <tr>
          <th class="ff-table-col-tiny" aria-hidden="true"></th>
          <th v-for="col in columns" :key="col.key">{{ col.label }}</th>
          <th class="ff-table-col-tiny" aria-hidden="true"></th>
        </tr>
      </thead>
      <draggable
        v-model="rows"
        tag="tbody"
        :group="dndScope"
        handle=".ff-table-handle"
        :animation="150"
        ghost-class="ff-table-row-ghost"
        chosen-class="ff-table-row-chosen"
        drag-class="ff-table-row-drag"
        :item-key="(_e: unknown[], i: number) => i"
      >
        <template #item="{ index: ri, element: row }">
          <tr>
            <td class="ff-table-col-tiny">
              <span
                class="ff-table-handle"
                :title="'Drag to reorder'"
                aria-hidden="true"
              >⠿</span>
            </td>
            <td v-for="(col, ci) in columns" :key="col.key + ':' + ci">
              <TextField
                v-if="col.type === 'string'"
                :model-value="asString(row[ci])"
                @update:model-value="(v) => setCell(ri, ci, v)"
                :readonly="field.readonly"
              />
              <TextField
                v-else-if="col.type === 'number'"
                type="number"
                :model-value="asString(row[ci])"
                @update:model-value="(v) => setCell(ri, ci, asNumber(v))"
                :readonly="field.readonly"
              />
              <TextField
                v-else-if="col.type === 'date'"
                type="date"
                :model-value="asString(row[ci])"
                @update:model-value="(v) => setCell(ri, ci, v)"
                :readonly="field.readonly"
              />
              <SwitchField
                v-else-if="col.type === 'bool'"
                :model-value="asBool(row[ci])"
                @update:model-value="(v) => setCell(ri, ci, v)"
                on-label="On"
                off-label="Off"
              />
              <SelectField
                v-else-if="col.type === 'dropdown'"
                :model-value="asString(row[ci])"
                @update:model-value="(v) => setCell(ri, ci, v)"
                :options="col.choices"
              />
            </td>
            <td class="ff-table-col-tiny">
              <button
                v-if="!field.readonly"
                type="button"
                class="ff-table-remove"
                :aria-label="'Remove row ' + (ri + 1)"
                @click="removeRow(ri)"
              >−</button>
            </td>
          </tr>
        </template>
      </draggable>
    </table>

    <p v-else class="muted small">
      No columns defined — set this field's options in the Templates editor.
    </p>

    <button
      v-if="!field.readonly && columns.length > 0"
      type="button"
      class="ff-table-add"
      @click="addRow"
    >+ Add row</button>
  </div>
</template>

<style scoped>
.table-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
}
.ff-table {
    width: 100%;
    border-collapse: collapse;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--color-bg);
}
.ff-table thead {
    background: var(--color-surface);
}
.ff-table th,
.ff-table td {
    border-bottom: 1px solid var(--color-border);
    padding: 4px 6px;
    text-align: left;
    font-size: var(--font-size-sm);
    vertical-align: middle;
}
.ff-table th {
    font-weight: 600;
    font-size: var(--font-size-sm);
    color: var(--color-text);
}
.ff-table tbody tr:last-child td { border-bottom: 0; }
.ff-table-col-tiny {
    width: 28px;
    text-align: center;
    padding: 4px 2px !important;
}
.ff-table-handle {
    cursor: grab;
    user-select: none;
    opacity: 0.6;
    font-size: 14px;
}
.ff-table-handle:active { cursor: grabbing; }

/* Sortable.js visual states (vuedraggable forwards class names). */
.ff-table-row-ghost {
    opacity: 0.35;
    filter: saturate(0.4);
}
.ff-table-row-chosen {
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
}
.ff-table-row-drag { cursor: grabbing; }
.ff-table-remove {
    appearance: none;
    width: 24px;
    height: 24px;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
    color: var(--color-text);
    border-radius: 4px;
    cursor: pointer;
    font-size: 12px;
    line-height: 1;
    font-weight: 600;
}
.ff-table-remove:hover { background: var(--color-surface-2); }

.ff-table-add {
    appearance: none;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
    color: var(--color-text);
    border-radius: var(--radius-md);
    cursor: pointer;
    padding: 6px;
    font-size: 14px;
    font-weight: 600;
    width: 100%;
}
.ff-table-add:hover { background: var(--color-surface-2); }

/* Inputs inside cells take full width and trim down padding so the
   table reads compact. */
.ff-table td :deep(.field-input) {
    padding: 4px 8px;
    width: 100%;
}
</style>
