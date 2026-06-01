<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ExportCsvRow, { type ExportRow, type SourceOption } from "./ExportCsvRow.vue";
import { SwitchField } from "./fields";
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

const includedRows = computed(() => rows.value.filter((r) => r.include));

// Master toggle for the Include column: on only when every row is included;
// flipping it sets all rows at once.
const allIncluded = computed({
  get: () => rows.value.length > 0 && rows.value.every((r) => r.include),
  set: (val: boolean) => rows.value.forEach((r) => (r.include = val)),
});
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
    const write = await CsvSvc.Write(path, result.rows, delimiter.value, true);
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
    scroll
    maximizable
    @close="emit('close')"
  >
    <template #head>
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
    </template>

    <table class="csv-import-table">
      <thead>
        <tr>
          <th class="csv-export-th-narrow">
            <div class="csv-export-include-head">
              <span>{{ t('csv.export.include') }}</span>
              <SwitchField v-model="allIncluded" />
            </div>
          </th>
          <th>{{ t('csv.export.source') }}</th>
          <th>{{ t('csv.column') }}</th>
          <th class="csv-export-th-narrow">{{ t('csv.export.separator') }}</th>
          <th>{{ t('csv.transform') }}</th>
          <th>{{ t('csv.preview') }}</th>
          <th class="csv-export-th-narrow"></th>
        </tr>
      </thead>
      <tbody>
        <ExportCsvRow
          v-for="(row, i) in rows"
          :key="row.id"
          :row="row"
          :source-options="sourceOptions"
          :label-by-key="labelByKey"
          :preview="row.include ? (previewCache[includedRows.indexOf(row)] ?? '') : ''"
          @remove="removeRow(i)"
        />
      </tbody>
    </table>

    <template #footer>
      <button type="button" class="tool-btn csv-export-add-computed" @click="addComputed">
        {{ t('csv.export.add.computed') }}
      </button>
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
