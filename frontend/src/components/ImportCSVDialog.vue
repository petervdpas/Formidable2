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

const file = ref("");
const delimiter = ref(",");
const preview = ref<PreviewResult | null>(null);
const mappings = ref<Mapping[]>([]);
const filenameField = ref("");
const concatSep = ref(" ");
const importing = ref(false);
const importError = ref("");

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

// Mappable fields derived from the template — excluded types stripped.
// FieldSpec instances are also what SuggestMappings / Coerce expect.
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

// Reset everything each time the dialog opens. The user may have
// changed the active template since the last open, and stale mappings
// against the old fields would be a silent footgun.
watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    file.value = "";
    delimiter.value = ",";
    preview.value = null;
    mappings.value = [];
    filenameField.value = "";
    concatSep.value = " ";
    importing.value = false;
    importError.value = "";
  },
);

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
// by both the "Preview" and "Transformed" columns in the table — they
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

// Reactive caches for the two preview columns — recomputed when
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
  let success = 0;
  let failed = 0;
  try {
    const total = preview.value.rows.length;
    for (let i = 0; i < total; i++) {
      const { data, stem } = await buildRowData(i);
      const filename = `${stem}.meta.json`;
      const r = await StorageSvc.ImportCsvRow(props.templateFilename, filename, data);
      if (r.success) success++;
      else failed++;
    }
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
</script>

<template>
  <Modal
    :open="open"
    :title="t('csv.import.title')"
    width="800px"
    maximizable
    :dialog-style="{ height: '600px' }"
    @close="emit('close')"
  >
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
    </div>

    <div v-if="importError" class="form-error">{{ importError }}</div>

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
              <option v-for="f in mappableFields" :key="f.key" :value="f.key">
                {{ f.label || f.key }} ({{ f.type }})
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

    <div v-if="preview && mappings.length" class="csv-import-bottom">
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
