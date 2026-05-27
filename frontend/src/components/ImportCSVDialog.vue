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
  ImportColumn,
  ImportPlan,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/csv";
import type { PreviewResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/csv";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
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

type Mapping = {
  header: string;
  fieldKey: string;
  rule: string;
  param: string;
};

type SourceOption = { value: string; label: string };

const file = ref("");
const delimiter = ref(",");
const preview = ref<PreviewResult | null>(null);
const mappings = ref<Mapping[]>([]);
const filenameField = ref("");
const concatSep = ref(" ");
const importing = ref(false);
const importError = ref("");

// Reversible (aligned) import: when alignSource names a list/table field,
// rows sharing groupKey collapse back into one entry with that field
// rebuilt - the inverse of the export's "Align rows on". subTargets are
// the dotted "table.subkey" targets the mapping dropdown then also offers.
const alignSource = ref("");
const alignable = ref<SourceOption[]>([]);
const subTargets = ref<SourceOption[]>([]);
const groupKey = ref("");

// Rules that expose a param input + their placeholder source.
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

// Re-parse whenever the delimiter flips while a file is already chosen.
watch(delimiter, async () => {
  if (file.value) await loadPreview();
});

async function pickFile() {
  const picked = await chooseFile([
    { displayName: "CSV", pattern: "*.csv" },
  ]);
  if (!picked) return;
  file.value = picked;
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
    const pr = await CsvSvc.Preview(file.value, delimiter.value);
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
  if (!preview.value || !hasMapping.value) return;
  importing.value = true;
  importError.value = "";
  try {
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
        <label class="muted small">{{ t('csv.delimiter') }}</label>
        <select v-model="delimiter">
          <option value=",">{{ t('csv.delimiter.comma') }}</option>
          <option value=";">{{ t('csv.delimiter.semicolon') }}</option>
          <option value="	">{{ t('csv.delimiter.tab') }}</option>
          <option value="|">{{ t('csv.delimiter.pipe') }}</option>
        </select>
      </div>
      <div v-if="alignable.length" class="csv-import-delim">
        <label class="muted small">{{ t('csv.import.align') }}</label>
        <select v-model="alignSource" @change="refreshAlign">
          <option value="">{{ t('csv.import.align.none') }}</option>
          <option v-for="o in alignable" :key="o.value" :value="o.value">
            {{ o.label }}
          </option>
        </select>
      </div>
      <div v-if="alignSource" class="csv-import-delim">
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

    <table v-if="preview && mappings.length" class="csv-import-table">
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
        <tr v-for="(m, i) in mappings" :key="i">
          <td class="csv-import-td-header">{{ m.header }}</td>
          <td>
            <select v-model="m.fieldKey">
              <option value="">{{ t('csv.skip') }}</option>
              <option v-for="o in targetOptions" :key="o.value" :value="o.value">
                {{ o.label }}
              </option>
            </select>
          </td>
          <td class="csv-import-td-transform">
            <select v-model="m.rule">
              <option v-for="r in transformRules" :key="r" :value="r">
                {{ t(transformLabelKey[r]) }}
              </option>
            </select>
            <input
              v-if="paramPlaceholder[m.rule] !== undefined"
              :type="paramInputType[m.rule] ?? 'text'"
              :placeholder="m.rule === 'bool-match' ? t('csv.transform.boolmatch.placeholder') : paramPlaceholder[m.rule]"
              v-model="m.param"
              class="csv-import-param"
            />
          </td>
          <td class="csv-import-td-preview muted small">
            {{ rawPreviewCache[i] ?? "" }}
          </td>
          <td class="csv-import-td-preview">
            {{ typedPreviewCache[i] ?? "" }}
          </td>
        </tr>
      </tbody>
    </table>

    <template v-if="preview && mappings.length" #foot>
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
        :disabled="!preview || !hasMapping || importing"
        @click="doImport"
      >
        {{ importing ? t('csv.importing') : t('csv.import') }}
      </button>
    </template>
  </Modal>
</template>
