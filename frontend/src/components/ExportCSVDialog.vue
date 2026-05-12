<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { useDialog } from "../composables/useDialog";
import { useToast } from "../composables/useToast";
import { backendErrMessage } from "../utils/backendError";
import {
  Service as CsvSvc,
  FieldSpec,
  ExportColumn,
  ExportPlan,
  Transform as ExpTransform,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/csv";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
  templateFilename: string;
  template: Template | null;
}>();
const emit = defineEmits<{
  (e: "close"): void;
  (e: "exported", count: number): void;
}>();

const { t } = useI18n();
const { chooseSaveFile } = useDialog();
const toast = useToast();

type ExportRow = {
  id: string;
  include: boolean;
  computed: boolean;
  header: string;
  sourceKeys: string[];
  separator: string;
  rule: string;
  param: string;
};

const delimiter = ref(",");
const alignSource = ref("");
const rows = ref<ExportRow[]>([]);
const sampleEntry = ref<Record<string, unknown> | null>(null);
const previewCache = ref<string[]>([]);
const exporting = ref(false);
const exportError = ref("");
const computedSeq = ref(0);

// Same transform metadata as the Import dialog. Kept in-file rather
// than shared so the dialogs stay independently editable.
const transformRules: string[] = [
  "none", "trim", "lowercase", "uppercase", "capitalize",
  "trim+lower", "trim+upper", "trim+cap",
  "first-n", "last-n", "split", "bool-match", "split-table",
];
const transformLabelKey: Record<string, string> = {
  "none": "csv.transform.none",
  "trim": "csv.transform.trim",
  "lowercase": "csv.transform.lowercase",
  "uppercase": "csv.transform.uppercase",
  "capitalize": "csv.transform.capitalize",
  "trim+lower": "csv.transform.trimlower",
  "trim+upper": "csv.transform.trimupper",
  "trim+cap": "csv.transform.trimcap",
  "first-n": "csv.transform.firstn",
  "last-n": "csv.transform.lastn",
  "split": "csv.transform.split",
  "bool-match": "csv.transform.boolmatch",
  "split-table": "csv.transform.splittable",
};
const paramPlaceholder: Record<string, string> = {
  "first-n": "N",
  "last-n": "N",
  "split": ", ; |",
  "bool-match": "",
  "split-table": "; ,",
};
const paramInputType: Record<string, "number" | "text"> = {
  "first-n": "number",
  "last-n": "number",
};

const mappableFields = computed<FieldSpec[]>(() => {
  const tpl = props.template;
  if (!tpl?.fields) return [];
  const excluded = new Set(["loopstart", "loopstop", "image", "code", "api"]);
  return tpl.fields
    .filter((f) => !excluded.has(f.type))
    .map((f) => FieldSpec.createFrom({
      key: f.key,
      type: f.type,
      label: f.label ?? "",
      options: f.options ?? [],
    }));
});

const fieldByKey = computed(() => {
  const m = new Map<string, FieldSpec>();
  for (const f of mappableFields.value) m.set(f.key, f);
  return m;
});

// Templates can declare list/table fields. Alignment only makes sense
// for those — the dropdown lists them so the user picks which (if any)
// field to unroll across rows.
const alignableFields = computed(() =>
  mappableFields.value.filter((f) => f.type === "list" || f.type === "table"),
);

function fieldLabel(key: string): string {
  const f = fieldByKey.value.get(key);
  if (!f) return key;
  return f.label ? `${f.label} (${f.type})` : `${f.key} (${f.type})`;
}

function defaultRows(): ExportRow[] {
  return mappableFields.value.map((f) => ({
    id: `dflt-${f.key}`,
    include: true,
    computed: false,
    header: f.key,
    sourceKeys: [f.key],
    separator: "",
    rule: "none",
    param: "",
  }));
}

// Reset everything when the dialog opens — the user may have switched
// templates since the last open, and stale rows against the old schema
// would mis-name columns.
watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    delimiter.value = ",";
    alignSource.value = "";
    rows.value = defaultRows();
    sampleEntry.value = null;
    previewCache.value = [];
    exporting.value = false;
    exportError.value = "";
    computedSeq.value = 0;
    await loadSample();
  },
);

async function loadSample() {
  if (!props.templateFilename) return;
  try {
    const files = await StorageSvc.ListForms(props.templateFilename);
    if (!files || files.length === 0) {
      sampleEntry.value = null;
      return;
    }
    const form = await StorageSvc.LoadForm(props.templateFilename, files[0]);
    sampleEntry.value = (form?.data as Record<string, unknown>) ?? null;
  } catch {
    sampleEntry.value = null;
  }
}

function addComputed() {
  computedSeq.value++;
  rows.value.push({
    id: `comp-${computedSeq.value}`,
    include: true,
    computed: true,
    header: t("csv.export.computed.default"),
    sourceKeys: [],
    separator: " ",
    rule: "none",
    param: "",
  });
}

function removeRow(idx: number) {
  rows.value.splice(idx, 1);
}

function addSourceToRow(row: ExportRow, fieldKey: string) {
  if (!fieldKey || row.sourceKeys.includes(fieldKey)) return;
  row.sourceKeys.push(fieldKey);
}

function removeSource(row: ExportRow, idx: number) {
  row.sourceKeys.splice(idx, 1);
}

const includedRows = computed(() => rows.value.filter((r) => r.include));
const canExport = computed(() =>
  includedRows.value.length > 0 && includedRows.value.every((r) => r.sourceKeys.length > 0),
);

// Build an ExportPlan from the current row state. The order of columns
// in the plan matches the order of rows in the UI; un-included rows
// drop out.
function buildPlan(): ExportPlan {
  const cols = includedRows.value.map((r) =>
    ExportColumn.createFrom({
      header: r.header || r.sourceKeys[0] || "column",
      sourceKeys: [...r.sourceKeys],
      separator: r.separator,
      transform: ExpTransform.createFrom({ rule: r.rule, param: r.param }),
    }),
  );
  return ExportPlan.createFrom({
    columns: cols,
    alignSource: alignSource.value,
  });
}

// Re-render the live preview whenever any column state changes. The
// preview is the first data row of BuildPreviewRows fed with one entry
// (or zero — then we just blank everything).
watch(
  [rows, alignSource, sampleEntry, mappableFields],
  async () => {
    if (!sampleEntry.value || includedRows.value.length === 0) {
      previewCache.value = includedRows.value.map(() => "");
      return;
    }
    try {
      const plan = buildPlan();
      const grid = await CsvSvc.BuildPreviewRows(plan, [sampleEntry.value], mappableFields.value);
      previewCache.value = grid.length > 1 ? grid[1] : grid[0]?.map(() => "") ?? [];
    } catch {
      previewCache.value = includedRows.value.map(() => "");
    }
  },
  { deep: true, immediate: true },
);

async function doExport() {
  if (!canExport.value) return;
  exporting.value = true;
  exportError.value = "";
  try {
    const tplStem = props.templateFilename.replace(/\.yaml$/, "");
    const path = await chooseSaveFile(`${tplStem}-export.csv`, [
      { displayName: "CSV", pattern: "*.csv" },
    ]);
    if (!path) {
      exporting.value = false;
      return;
    }
    const plan = buildPlan();
    const result = await CsvSvc.Export(props.templateFilename, plan, mappableFields.value);
    if (result.error) {
      exportError.value = result.error;
      toast.error("csv.export.failed");
      return;
    }
    const write = await CsvSvc.Write(path, result.rows, delimiter.value);
    if (!write.success) {
      exportError.value = write.error || t("csv.export.failed");
      toast.error("csv.export.failed");
      return;
    }
    toast.success("csv.export.success", [result.count]);
    emit("exported", result.count);
    emit("close");
  } catch (e) {
    exportError.value = backendErrMessage(e);
    toast.error("csv.export.failed");
  } finally {
    exporting.value = false;
  }
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('csv.export.title')"
    width="800px"
    maximizable
    @close="emit('close')"
  >
    <div class="csv-import-target">
      <span class="csv-import-target-label">{{ t('csv.template') }}:</span>
      <code class="csv-import-target-value">{{ template?.name || templateFilename }}</code>
    </div>

    <div class="csv-export-toprow">
      <div class="csv-export-field-group">
        <label class="csv-export-field-label">{{ t('csv.delimiter') }}</label>
        <select v-model="delimiter">
          <option value=",">{{ t('csv.delimiter.comma') }}</option>
          <option value=";">{{ t('csv.delimiter.semicolon') }}</option>
          <option value="	">{{ t('csv.delimiter.tab') }}</option>
          <option value="|">{{ t('csv.delimiter.pipe') }}</option>
        </select>
      </div>
      <div class="csv-export-field-group">
        <label class="csv-export-field-label">{{ t('csv.export.align') }}</label>
        <select v-model="alignSource">
          <option value="">{{ t('csv.export.align.none') }}</option>
          <option v-for="f in alignableFields" :key="f.key" :value="f.key">
            {{ fieldLabel(f.key) }}
          </option>
        </select>
      </div>
    </div>

    <div v-if="exportError" class="form-error">{{ exportError }}</div>

    <table class="csv-import-table">
      <thead>
        <tr>
          <th class="csv-export-th-narrow">{{ t('csv.export.include') }}</th>
          <th>{{ t('csv.export.source') }}</th>
          <th>{{ t('csv.column') }}</th>
          <th class="csv-export-th-narrow">{{ t('csv.export.separator') }}</th>
          <th>{{ t('csv.transform') }}</th>
          <th>{{ t('csv.preview') }}</th>
          <th class="csv-export-th-narrow"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, i) in rows" :key="row.id">
          <td class="csv-export-td-narrow">
            <input type="checkbox" v-model="row.include" />
          </td>
          <td>
            <template v-if="!row.computed">
              <span class="muted small">{{ fieldLabel(row.sourceKeys[0]) }}</span>
            </template>
            <template v-else>
              <div class="csv-export-chips">
                <span v-for="(key, ki) in row.sourceKeys" :key="ki" class="csv-export-chip">
                  {{ fieldLabel(key) }}
                  <button
                    type="button"
                    class="csv-export-chip-x"
                    @click="removeSource(row, ki)"
                    :aria-label="t('common.remove')"
                  >×</button>
                </span>
                <select
                  class="csv-export-chip-add"
                  :value="''"
                  @change="addSourceToRow(row, ($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''"
                >
                  <option value="">{{ t('csv.export.add.field') }}</option>
                  <option v-for="f in mappableFields" :key="f.key" :value="f.key">
                    {{ fieldLabel(f.key) }}
                  </option>
                </select>
              </div>
            </template>
          </td>
          <td>
            <input v-model="row.header" class="csv-export-header-input" />
          </td>
          <td class="csv-export-td-narrow">
            <span v-if="!row.computed" class="muted">—</span>
            <input v-else v-model="row.separator" class="csv-import-concat-input" />
          </td>
          <td class="csv-import-td-transform">
            <select v-model="row.rule">
              <option v-for="r in transformRules" :key="r" :value="r">
                {{ t(transformLabelKey[r]) }}
              </option>
            </select>
            <input
              v-if="paramPlaceholder[row.rule] !== undefined"
              :type="paramInputType[row.rule] ?? 'text'"
              :placeholder="row.rule === 'bool-match' ? t('csv.transform.boolmatch.placeholder') : paramPlaceholder[row.rule]"
              v-model="row.param"
              class="csv-import-param"
            />
          </td>
          <td class="csv-import-td-preview muted small">
            {{ row.include ? (previewCache[includedRows.indexOf(row)] ?? "") : "" }}
          </td>
          <td class="csv-export-td-narrow">
            <button
              v-if="row.computed"
              type="button"
              class="csv-export-row-x"
              :aria-label="t('common.remove')"
              @click="removeRow(i)"
            >×</button>
          </td>
        </tr>
      </tbody>
    </table>

    <div class="csv-export-actions">
      <button type="button" class="tool-btn" @click="addComputed">
        {{ t('csv.export.add.computed') }}
      </button>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canExport || exporting"
        @click="doExport"
      >
        {{ exporting ? t('csv.exporting') : t('csv.export') }}
      </button>
    </template>
  </Modal>
</template>
