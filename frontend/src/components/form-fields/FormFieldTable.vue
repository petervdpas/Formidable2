<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import { TextField, SelectField, SwitchField, DateInput, type SelectOption } from "../fields";
import PasteDataDialog from "../PasteDataDialog.vue";
import { useConfig } from "../../composables/useConfig";
import {
  Service as CsvSvc,
  TableColumn,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/csv";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const { t } = useI18n();
const { config } = useConfig();
const showPaste = computed(() => !!config.value?.show_paste_buttons);
const pasteOpen = ref(false);

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
// "reference" is deferred to a follow-up.

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

async function onPasteProcess(pasted: string[][]) {
  pasteOpen.value = false;
  if (pasted.length === 0 || columns.value.length === 0) return;
  const specs: TableColumn[] = columns.value.map((c) =>
    new TableColumn({ type: c.type, choices: c.choices as unknown as any[] }),
  );
  const typed = await CsvSvc.CoerceTableRows(specs, pasted);
  if (typed.length > 0) {
    emitRows([...rows.value, ...typed]);
  }
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
        handle=".dnd-handle"
        :animation="150"
        ghost-class="dnd-ghost"
        chosen-class="dnd-chosen"
        drag-class="dnd-drag"
        :item-key="(_e: unknown[], i: number) => i"
      >
        <template #item="{ index: ri, element: row }">
          <tr>
            <td class="ff-table-col-tiny">
              <span
                class="dnd-handle"
                :title="t('workspace.storage.field.drag_to_reorder')"
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
              <DateInput
                v-else-if="col.type === 'date'"
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
                class="btn-ghost-icon btn-sm"
                :aria-label="t('workspace.storage.field.remove_row')"
                @click="removeRow(ri)"
              >−</button>
            </td>
          </tr>
        </template>
      </draggable>
    </table>

    <p v-else class="muted small">
      {{ t('workspace.storage.field.table_no_columns') }}
    </p>

    <div v-if="!field.readonly && columns.length > 0" class="table-actions">
      <button
        type="button"
        class="btn-ghost-icon"
        :aria-label="t('workspace.storage.field.add_row')"
        :title="t('workspace.storage.field.add_row')"
        @click="addRow"
      >+</button>
      <button
        v-if="showPaste"
        type="button"
        class="btn-ghost-icon"
        :aria-label="t('paste.tooltip')"
        :title="t('paste.tooltip')"
        @click="pasteOpen = true"
      ><i class="fa-solid fa-paste"></i></button>
    </div>

    <PasteDataDialog
      :open="pasteOpen"
      :title="t('paste.table.title')"
      :subtitle="t('paste.table.subtitle')"
      @process="onPasteProcess"
      @cancel="pasteOpen = false"
    />
  </div>
</template>

