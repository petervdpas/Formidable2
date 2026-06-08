<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ImportCsvRow, { type Mapping } from "./ImportCsvRow.vue";
import { useDialog } from "../composables/useDialog";
import { useToast } from "../composables/useToast";
import { backendErrMessage } from "../utils/backendError";
import {
  Service as CsvSvc,
  FieldSpec,
  ImportColumn,
  ImportPlan,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/csv";
import type { PreviewResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/csv";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import {
  Service as FormSvc,
  EdgePair,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import type { Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
  templateFilename: string;
  template: Template | null;
}>();
const emit = defineEmits<{
  (e: "close"): void;
  (e: "imported", count: number): void;
}>();

const { t } = useI18n();
const { chooseFile } = useDialog();
const toast = useToast();

type SourceOption = { value: string; label: string };

const file = ref("");
const delimiter = ref(",");
const preview = ref<PreviewResult | null>(null);
const mappings = ref<Mapping[]>([]);
const filenameField = ref("");
const concatSep = ref(" ");
const importing = ref(false);
const importError = ref("");

// Source can be a CSV (delimiter-driven) or one sheet of an .xlsx workbook
// (sheet picker). isExcel switches the read path; both feed the same headers
// + rows preview so the mapping pipeline below is source-agnostic.
const sheets = ref<string[]>([]);
const sheet = ref("");
const isExcel = computed(() => file.value.toLowerCase().endsWith(".xlsx"));

// Two import modes. "records" is the original CSV->entry import. "relations"
// reads two id columns (source guid, target guid) and links them through an
// api field, writing the picks onto the existing source records so the
// reference-edge syncer mirrors them into the relation graph. Run records
// first (so both endpoints exist), then relations.
type ImportMode = "records" | "relations";
const mode = ref<ImportMode>("records");
const relationFieldKey = ref("");
const fromColumn = ref("");
const toColumn = ref("");

// The template's api fields are the relation targets a relations-import can
// fill. Read straight off the loaded template (backend owns field shape).
const apiFields = computed<SourceOption[]>(() =>
  (props.template?.fields ?? [])
    .filter((f) => f.type === "api")
    .map((f) => ({
      value: f.key,
      label: `${f.label || f.key} -> ${f.collection || "?"}`,
    })),
);

const headerOptions = computed<SourceOption[]>(() =>
  (preview.value?.headers ?? []).map((h) => ({ value: h, label: h })),
);

// Reversible (aligned) import: when alignSource names a list/table field,
// rows sharing groupKey collapse back into one entry with that field
// rebuilt - the inverse of the export's "Align rows on". subTargets are
// the dotted "table.subkey" targets the mapping dropdown then also offers.
const alignSource = ref("");
const alignable = ref<SourceOption[]>([]);
const subTargets = ref<SourceOption[]>([]);
const groupKey = ref("");

// Mappable fields come from the backend (excluded types stripped there,
// one source of truth). The dialog still needs the FieldSpec list locally
// to drive the mapping dropdown and per-cell coerce previews.
const mappableFields = ref<FieldSpec[]>([]);

const fieldByKey = computed(() => {
  const m = new Map<string, FieldSpec>();
  for (const f of mappableFields.value) m.set(f.key, f);
  return m;
});

// Reset everything each time the dialog opens. The user may have
// changed the active template since the last open, and stale mappings
// against the old fields would be a silent footgun.
watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    file.value = "";
    delimiter.value = ",";
    preview.value = null;
    mappings.value = [];
    filenameField.value = "";
    concatSep.value = " ";
    importing.value = false;
    importError.value = "";
    mappableFields.value = [];
    alignSource.value = "";
    alignable.value = [];
    subTargets.value = [];
    groupKey.value = "";
    sheets.value = [];
    sheet.value = "";
    mode.value = "records";
    relationFieldKey.value = apiFields.value[0]?.value ?? "";
    fromColumn.value = "";
    toColumn.value = "";
    try {
      mappableFields.value = await CsvSvc.MappableFieldsForTemplate(props.templateFilename);
      // Default the group key to the template's guid field (the export
      // repeats it on every aligned row), falling back to the first field.
      const guid = mappableFields.value.find((f) => f.type === "guid");
      groupKey.value = guid?.key ?? mappableFields.value[0]?.key ?? "";
      await refreshAlign();
    } catch (e) {
      importError.value = backendErrMessage(e);
    }
  },
);

// Fetch the alignable fields and, when a table is the align target, the
// dotted "table.subkey" targets - reusing the export schema so both
// dialogs share one contract. Called on open and on each align change.
async function refreshAlign() {
  if (!props.templateFilename) return;
  try {
    const schema = await CsvSvc.ExportSchema(props.templateFilename, alignSource.value);
    if (schema.error) return;
    alignable.value = (schema.alignable ?? []).map((o) => ({ value: o.value, label: o.label }));
    alignSource.value = schema.plan?.alignSource ?? "";
    const prefix = alignSource.value ? `${alignSource.value}.` : "";
    subTargets.value = prefix
      ? (schema.sources ?? [])
          .filter((o) => o.value.startsWith(prefix))
          .map((o) => ({ value: o.value, label: o.label }))
      : [];
  } catch (e) {
    importError.value = backendErrMessage(e);
  }
}

// The mapping target dropdown offers the flat mappable fields plus, when
// aligned on a table, that table's dotted column targets.
const targetOptions = computed<SourceOption[]>(() => {
  const flat = mappableFields.value.map((f) => ({
    value: f.key,
    label: `${f.label || f.key} (${f.type})`,
  }));
  return [...flat, ...subTargets.value];
});

// Re-parse whenever the delimiter (CSV) or sheet (Excel) changes.
watch(delimiter, async () => {
  if (file.value && !isExcel.value) await loadPreview();
});
watch(sheet, async () => {
  if (file.value && isExcel.value) await loadPreview();
});

async function pickFile() {
  const picked = await chooseFile([
    // GTK's glob matches case-sensitively and supports no char classes, so
    // enumerate lower- and upper-case extensions (e.g. a ".XLSX" export).
    { displayName: "Spreadsheet (CSV, Excel)", pattern: "*.csv;*.CSV;*.xlsx;*.XLSX" },
  ]);
  if (!picked) return;
  file.value = picked;
  if (isExcel.value) {
    try {
      sheets.value = await CsvSvc.SheetNames(file.value);
      sheet.value = sheets.value[0] ?? "";
    } catch (e) {
      importError.value = backendErrMessage(e);
      return;
    }
  } else {
    sheets.value = [];
    sheet.value = "";
  }
  await loadPreview();
}

async function loadPreview() {
  if (!file.value) {
    preview.value = null;
    mappings.value = [];
    return;
  }
  importError.value = "";
  try {
    const pr = isExcel.value
      ? await CsvSvc.PreviewSheet(file.value, sheet.value)
      : await CsvSvc.Preview(file.value, delimiter.value);
    if (pr.error) {
      importError.value = t("csv.error.parse", [pr.error]);
      preview.value = null;
      mappings.value = [];
      return;
    }
    preview.value = pr;
    const suggested = await CsvSvc.SuggestMappings(pr.headers, mappableFields.value);
    mappings.value = suggested.map((s) => ({
      header: s.header,
      fieldKey: s.fieldKey,
      rule: "none",
      param: "",
    }));
    filenameField.value = "";
    // Default the relation columns to the first two headers as a hint.
    fromColumn.value = pr.headers[0] ?? "";
    toColumn.value = pr.headers[1] ?? pr.headers[0] ?? "";
  } catch (e) {
    importError.value = t("csv.error.parse", [backendErrMessage(e)]);
    preview.value = null;
    mappings.value = [];
  }
}

// Live preview: transform the first data row's cell for this mapping,
// then run it through CoercePreview against the bound field type. Used
// by both the "Preview" and "Transformed" columns in the table - they
// show raw-cell and typed-cell respectively for the first row only.
async function previewCell(mapping: Mapping, mode: "raw" | "typed"): Promise<string> {
  const pr = preview.value;
  if (!pr || pr.rows.length === 0) return "";
  const idx = pr.headers.indexOf(mapping.header);
  if (idx === -1) return "";
  const raw = (pr.rows[0]?.[idx] ?? "");
  if (mode === "raw") return raw;
  const transformed = await CsvSvc.ApplyTransform(raw, mapping.rule, mapping.param, "preview");
  const field = fieldByKey.value.get(mapping.fieldKey);
  if (!field) return transformed;
  return CsvSvc.CoercePreview(transformed, field.type, field.options ?? []);
}

// Reactive caches for the two preview columns - recomputed when
// mappings / preview / fields change. Promise rendering via async
// methods would force template suspense; caching keeps the table
// fully synchronous on render.
const rawPreviewCache = ref<string[]>([]);
const typedPreviewCache = ref<string[]>([]);

watch(
  [mappings, preview, fieldByKey],
  async () => {
    const raws = await Promise.all(mappings.value.map((m) => previewCell(m, "raw")));
    const typed = await Promise.all(mappings.value.map((m) => previewCell(m, "typed")));
    rawPreviewCache.value = raws;
    typedPreviewCache.value = typed;
  },
  { deep: true, immediate: true },
);

const hasMapping = computed(() => mappings.value.some((m) => m.fieldKey));

const canImport = computed(() => {
  if (!preview.value) return false;
  if (mode.value === "relations") {
    return (
      !!relationFieldKey.value &&
      !!fromColumn.value &&
      !!toColumn.value &&
      fromColumn.value !== toColumn.value
    );
  }
  return hasMapping.value;
});

// "Derive filename from" options: auto + any CSV header.
const filenameOptions = computed(() => {
  const opts = [{ value: "", label: t("csv.auto.filename") }];
  if (preview.value) {
    for (const h of preview.value.headers) opts.push({ value: h, label: h });
  }
  return opts;
});

function sanitizeStem(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
}

async function buildRowData(rowIdx: number): Promise<{ data: Record<string, unknown>; stem: string }> {
  const pr = preview.value!;
  const row = pr.rows[rowIdx];
  const headerIdx = (h: string) => pr.headers.indexOf(h);

  // Group mappings by target field key.
  const byField = new Map<string, Mapping[]>();
  for (const m of mappings.value) {
    if (!m.fieldKey) continue;
    const list = byField.get(m.fieldKey) ?? [];
    list.push(m);
    byField.set(m.fieldKey, list);
  }

  const data: Record<string, unknown> = {};
  for (const [fieldKey, mrows] of byField.entries()) {
    const field = fieldByKey.value.get(fieldKey);
    if (!field) continue;
    const parts: string[] = [];
    for (const mr of mrows) {
      const raw = row[headerIdx(mr.header)] ?? "";
      const transformed = await CsvSvc.ApplyTransform(raw, mr.rule, mr.param, "storage");
      parts.push(transformed);
    }
    const joined = parts.length === 1 ? parts[0] : parts.join(concatSep.value);
    data[fieldKey] = await CsvSvc.CoerceValue(joined, field.type, field.options ?? []);
  }

  let stem = "";
  if (filenameField.value) {
    const idx = headerIdx(filenameField.value);
    if (idx !== -1) stem = sanitizeStem(row[idx] ?? "");
  }
  if (!stem) stem = `import-${rowIdx + 1}-${Date.now()}`;
  return { data, stem };
}

async function doImport() {
  if (!canImport.value) return;
  importing.value = true;
  importError.value = "";
  try {
    if (mode.value === "relations") {
      await importRelations();
      emit("close");
      return;
    }
    const { success, failed } = alignSource.value
      ? await importAligned()
      : await importPerRow();
    if (failed === 0) {
      toast.success("csv.import.success", [success]);
    } else {
      toast.error("csv.import.failed");
    }
    emit("imported", success);
    emit("close");
  } catch (e) {
    importError.value = backendErrMessage(e);
  } finally {
    importing.value = false;
  }
}

// Relations mode: read the two id columns into {from,to} pairs and let the
// backend group them by source, union them onto each source record's api
// field, and sync the edges. Run after the record import so both endpoints
// already exist on disk.
async function importRelations(): Promise<void> {
  const pr = preview.value!;
  const fromIdx = pr.headers.indexOf(fromColumn.value);
  const toIdx = pr.headers.indexOf(toColumn.value);
  const pairs: EdgePair[] = [];
  for (const row of pr.rows) {
    const from = (row[fromIdx] ?? "").trim();
    const to = (row[toIdx] ?? "").trim();
    if (from && to) pairs.push(EdgePair.createFrom({ from, to }));
  }
  const res = await FormSvc.ImportRelationEdges(
    props.templateFilename,
    relationFieldKey.value,
    pairs,
  );
  const skipped = res.missingFrom + res.missingTo;
  if (skipped > 0) {
    toast.error("csv.import.relation.partial", [res.linked, res.records, skipped]);
  } else {
    toast.success("csv.import.relation.success", [res.linked, res.records]);
  }
  emit("imported", res.linked);
}

// Unaligned: one entry per CSV row (the original behaviour).
async function importPerRow(): Promise<{ success: number; failed: number }> {
  let success = 0;
  let failed = 0;
  const total = preview.value!.rows.length;
  for (let i = 0; i < total; i++) {
    const { data, stem } = await buildRowData(i);
    const r = await StorageSvc.ImportCsvRow(props.templateFilename, `${stem}.meta.json`, data);
    if (r.success) success++;
    else failed++;
  }
  return { success, failed };
}

// Aligned: hand the whole sheet to the backend, which regroups the
// multiplied rows back into one entry per group with the nested
// list/table rebuilt, then save each reconstructed entry.
async function importAligned(): Promise<{ success: number; failed: number }> {
  const pr = preview.value!;
  const columns = mappings.value
    .filter((m) => m.fieldKey)
    .map((m) =>
      ImportColumn.createFrom({
        header: m.header,
        target: m.fieldKey,
        transform: { rule: m.rule, param: m.param },
      }),
    );
  const plan = ImportPlan.createFrom({
    columns,
    alignSource: alignSource.value,
    groupKey: groupKey.value,
  });
  const forms = await CsvSvc.BuildImportForms(plan, pr.headers, pr.rows, mappableFields.value);
  let success = 0;
  let failed = 0;
  for (let i = 0; i < forms.length; i++) {
    const f = forms[i];
    const stem = sanitizeStem(f.key ?? "") || `import-${i + 1}-${Date.now()}`;
    const r = await StorageSvc.ImportCsvRow(
      props.templateFilename,
      `${stem}.meta.json`,
      f.data as Record<string, unknown>,
    );
    if (r.success) success++;
    else failed++;
  }
  return { success, failed };
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('csv.import.title')"
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

    <div class="csv-import-toprow">
      <div class="csv-import-file">
        <button class="tool-btn" type="button" @click="pickFile">
          {{ t('csv.choose.file') }}
        </button>
        <span class="muted small">{{ file || t('csv.no.file') }}</span>
      </div>
      <div class="csv-import-delim">
        <label class="muted small">{{ t('csv.import.mode') }}</label>
        <select v-model="mode">
          <option value="records">{{ t('csv.import.mode.records') }}</option>
          <option value="relations">{{ t('csv.import.mode.relations') }}</option>
        </select>
      </div>
      <div v-if="isExcel && sheets.length" class="csv-import-delim">
        <label class="muted small">{{ t('csv.sheet') }}</label>
        <select v-model="sheet">
          <option v-for="s in sheets" :key="s" :value="s">{{ s }}</option>
        </select>
      </div>
      <div v-if="!isExcel" class="csv-import-delim">
        <label class="muted small">{{ t('csv.delimiter') }}</label>
        <select v-model="delimiter">
          <option value=",">{{ t('csv.delimiter.comma') }}</option>
          <option value=";">{{ t('csv.delimiter.semicolon') }}</option>
          <option value="	">{{ t('csv.delimiter.tab') }}</option>
          <option value="|">{{ t('csv.delimiter.pipe') }}</option>
        </select>
      </div>
      <div v-if="mode === 'records' && alignable.length" class="csv-import-delim">
        <label class="muted small">{{ t('csv.import.align') }}</label>
        <select v-model="alignSource" @change="refreshAlign">
          <option value="">{{ t('csv.import.align.none') }}</option>
          <option v-for="o in alignable" :key="o.value" :value="o.value">
            {{ o.label }}
          </option>
        </select>
      </div>
      <div v-if="mode === 'records' && alignSource" class="csv-import-delim">
        <label class="muted small">{{ t('csv.import.group') }}</label>
        <select v-model="groupKey">
          <option v-for="f in mappableFields" :key="f.key" :value="f.key">
            {{ f.label || f.key }}
          </option>
        </select>
      </div>
    </div>

    <div v-if="importError" class="form-error">{{ importError }}</div>
    </template>

    <table v-if="mode === 'records' && preview && mappings.length" class="csv-import-table">
      <thead>
        <tr>
          <th>{{ t('csv.column') }}</th>
          <th>{{ t('csv.field') }}</th>
          <th>{{ t('csv.transform') }}</th>
          <th>{{ t('csv.preview') }}</th>
          <th>{{ t('csv.transformed') }}</th>
        </tr>
      </thead>
      <tbody>
        <ImportCsvRow
          v-for="(m, i) in mappings"
          :key="i"
          :mapping="m"
          :target-options="targetOptions"
          :raw-preview="rawPreviewCache[i] ?? ''"
          :typed-preview="typedPreviewCache[i] ?? ''"
        />
      </tbody>
    </table>

    <div v-if="mode === 'relations' && preview" class="csv-import-relations">
      <p class="muted small">{{ t('csv.import.relation.hint') }}</p>
      <p v-if="!apiFields.length" class="form-error">
        {{ t('csv.import.relation.none') }}
      </p>
      <template v-else>
        <div class="csv-import-delim">
          <label class="muted small">{{ t('csv.import.relation.field') }}</label>
          <select v-model="relationFieldKey">
            <option v-for="o in apiFields" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </select>
        </div>
        <div class="csv-import-delim">
          <label class="muted small">{{ t('csv.import.relation.from') }}</label>
          <select v-model="fromColumn">
            <option v-for="o in headerOptions" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </select>
        </div>
        <div class="csv-import-delim">
          <label class="muted small">{{ t('csv.import.relation.to') }}</label>
          <select v-model="toColumn">
            <option v-for="o in headerOptions" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </select>
        </div>
      </template>
    </div>

    <template v-if="mode === 'records' && preview && mappings.length" #foot>
    <div class="csv-import-bottom">
      <div class="csv-import-concat muted small">
        <span>{{ t('csv.concat.hint') }}</span>
        <label>
          {{ t('csv.concat.separator') }}
          <input
            v-model="concatSep"
            class="csv-import-concat-input"
            :title="t('csv.concat.separator.title')"
          />
        </label>
      </div>
      <div class="csv-import-filename">
        <label>{{ t('csv.filename.field') }}</label>
        <select v-model="filenameField">
          <option v-for="o in filenameOptions" :key="o.value" :value="o.value">
            {{ o.label }}
          </option>
        </select>
      </div>
      <div class="muted small">
        {{ t('csv.rows.found', [preview.rowCount]) }}
      </div>
    </div>
    </template>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canImport || importing"
        @click="doImport"
      >
        {{ importing ? t('csv.importing') : t('csv.import') }}
      </button>
    </template>
  </Modal>
</template>
