<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { useDialog } from "../composables/useDialog";
import { useToast } from "../composables/useToast";
import { backendErrMessage } from "../utils/backendError";
import {
  Service as CsvSvc,
  ExportColumn,
  ExportPlan,
  Transform as ExpTransform,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/csv";
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

type SourceOption = { value: string; label: string };

const delimiter = ref(",");
const alignSource = ref("");
const rows = ref<ExportRow[]>([]);
const alignable = ref<SourceOption[]>([]);
const sourceOptions = ref<SourceOption[]>([]);
const labelByKey = ref<Map<string, string>>(new Map());
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

function fieldLabel(key: string): string {
  return labelByKey.value.get(key) ?? key;
}

// The backend owns the column rules: which field types are exportable,
// which fields are alignable, and how an aligned table expands into dotted
// "table.column" keys. refreshSchema fetches that schema for the current
// alignment and rebuilds the default rows from it. Computed columns the
// user added are preserved across an alignment change.
async function refreshSchema(preserveComputed: boolean) {
  if (!props.templateFilename) return;
  const userComputed = preserveComputed ? rows.value.filter((r) => r.computed) : [];
  try {
    const schema = await CsvSvc.ExportSchema(props.templateFilename, alignSource.value);
    if (schema.error) {
      exportError.value = schema.error;
      return;
    }
    alignable.value = (schema.alignable ?? []).map((o) => ({ value: o.value, label: o.label }));
    sourceOptions.value = (schema.sources ?? []).map((o) => ({ value: o.value, label: o.label }));
    const labels = new Map<string, string>();
    for (const o of sourceOptions.value) labels.set(o.value, o.label);
    for (const o of alignable.value) labels.set(o.value, o.label);
    labelByKey.value = labels;
    // Echo the backend's validated alignSource (clears an unknown pick).
    alignSource.value = schema.plan?.alignSource ?? "";
    const defaults: ExportRow[] = (schema.plan?.columns ?? []).map((c) => ({
      id: `dflt-${c.sourceKeys[0] ?? c.header}`,
      include: true,
      computed: false,
      header: c.header,
      sourceKeys: [...c.sourceKeys],
      separator: c.separator ?? "",
      rule: c.transform?.rule || "none",
      param: c.transform?.param || "",
    }));
    rows.value = [...defaults, ...userComputed];
  } catch (e) {
    exportError.value = backendErrMessage(e);
  }
}

// Reset everything when the dialog opens - the user may have switched
// templates since the last open, and stale rows against the old schema
// would mis-name columns.
watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    delimiter.value = ",";
    alignSource.value = "";
    rows.value = [];
    previewCache.value = [];
    exporting.value = false;
    exportError.value = "";
    computedSeq.value = 0;
    await refreshSchema(false);
  },
);

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
// backend builds the first data row from the template's first stored form
// (PreviewExport); cells line up with the included columns in order.
watch(
  [rows, () => props.templateFilename],
  async () => {
    if (!props.templateFilename || includedRows.value.length === 0) {
      previewCache.value = includedRows.value.map(() => "");
      return;
    }
    try {
      const res = await CsvSvc.PreviewExport(props.templateFilename, buildPlan());
      previewCache.value = res.cells ?? includedRows.value.map(() => "");
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
    const result = await CsvSvc.Export(props.templateFilename, plan);
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
        <select v-model="alignSource" @change="refreshSchema(true)">
          <option value="">{{ t('csv.export.align.none') }}</option>
          <option v-for="o in alignable" :key="o.value" :value="o.value">
            {{ o.label }}
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
                  <option v-for="o in sourceOptions" :key="o.value" :value="o.value">
                    {{ o.label }}
                  </option>
                </select>
              </div>
            </template>
          </td>
          <td>
            <input v-model="row.header" class="csv-export-header-input" />
          </td>
          <td class="csv-export-td-narrow">
            <span v-if="!row.computed" class="muted">-</span>
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
