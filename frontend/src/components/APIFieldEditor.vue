<script setup lang="ts">
// Editor for the api field's two pieces of config:
//   • collection — the SOURCE template (filename) the field references.
//                  Restricted to collection-enabled templates so the
//                  picker can address records by guid.
//   • map        — the column list. Each entry projects one level-0
//                  source field into the host form's row at fetch time.
//                  Type is read-only (resolved live from the source
//                  template) so a source-side rename or type change
//                  can't drift a stale cache here.
//
// Surface intentionally narrow: no use_picker / allowed_ids /
// id-input / map.path/mode any more — those collapsed in Slice 1
// once the design settled on "always pick by guid".

import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { SelectField, TextField } from "./fields";
import {
  Service as TemplateSvc,
  type Field,
  APIMap,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as DataproviderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";

const props = defineProps<{
  /** Bound field draft. We mutate `collection` and `map` directly so
   *  FieldEditModal's deep-copy-on-open / commit-on-confirm cycle
   *  Just Works without per-attribute emits. */
  field: Field;
}>();

const { t } = useI18n();

// ── Source template list (collection-enabled only) ─────────────────────
const sources = ref<{ filename: string; name: string }[]>([]);
const sourcesLoading = ref(false);
const sourcesError = ref("");

async function loadSources() {
  sourcesLoading.value = true;
  sourcesError.value = "";
  try {
    const list = (await DataproviderSvc.ListCollectionTemplates()) ?? [];
    sources.value = list.map((s) => ({
      filename: s.filename,
      name: s.name || s.filename,
    }));
  } catch (e) {
    sourcesError.value = String((e as Error)?.message ?? e);
    sources.value = [];
  } finally {
    sourcesLoading.value = false;
  }
}

void loadSources();

const sourceOptions = computed(() =>
  sources.value.map((s) => ({ value: s.filename, label: s.name })),
);

// ── Source template's level-0 field roster ─────────────────────────────
//
// Loaded whenever `field.collection` changes. Inside-loop fields are
// excluded — Map[] entries can only project top-level fields.
type SourceField = { key: string; label: string; type: string };
const sourceFields = ref<SourceField[]>([]);
const sourceFieldsLoading = ref(false);
const sourceFieldsError = ref("");

async function loadSourceFields(filename: string) {
  if (!filename) {
    sourceFields.value = [];
    return;
  }
  sourceFieldsLoading.value = true;
  sourceFieldsError.value = "";
  try {
    const tpl = await TemplateSvc.LoadTemplate(filename);
    if (!tpl) {
      sourceFields.value = [];
      return;
    }
    sourceFields.value = topLevelFields(tpl.fields ?? []);
  } catch (e) {
    sourceFieldsError.value = String((e as Error)?.message ?? e);
    sourceFields.value = [];
  } finally {
    sourceFieldsLoading.value = false;
  }
}

watch(
  () => props.field.collection,
  (collection) => {
    void loadSourceFields(collection ?? "");
  },
  { immediate: true },
);

// Walk the field roster, skipping anything between a loopstart and its
// matching loopstop. Marker types (loopstart/loopstop/looper/guid) are
// also excluded — guid is automatic on stamp; the loop markers are
// containers, not projectable fields.
function topLevelFields(fields: Field[]): SourceField[] {
  const out: SourceField[] = [];
  let depth = 0;
  for (const f of fields) {
    if (f.type === "loopstart") {
      depth++;
      continue;
    }
    if (f.type === "loopstop") {
      if (depth > 0) depth--;
      continue;
    }
    if (depth > 0) continue;
    if (f.type === "looper" || f.type === "guid") continue;
    out.push({
      key: f.key,
      label: f.label || f.key,
      type: f.type || "text",
    });
  }
  return out;
}

const sourceFieldOptions = computed(() =>
  sourceFields.value.map((f) => ({
    value: f.key,
    label: f.label === f.key ? f.key : `${f.label} (${f.key})`,
  })),
);

function typeOf(key: string): string {
  return sourceFields.value.find((f) => f.key === key)?.type ?? "";
}

// ── Map[] mutators ─────────────────────────────────────────────────────
//
// The api editor mutates props.field.map directly. We keep helpers
// here so the template stays light and the array reference stays
// stable for Vue's reactivity.

function ensureMap(): APIMap[] {
  if (!Array.isArray(props.field.map)) {
    props.field.map = [];
  }
  return props.field.map;
}

function addRow() {
  ensureMap().push(APIMap.createFrom({ key: "", label: "" }));
}

function removeRow(idx: number) {
  ensureMap().splice(idx, 1);
}
</script>

<template>
  <div class="api-field-editor">
    <!-- Source template -->
    <div class="api-field-row">
      <label class="api-field-label">
        {{ t("workspace.templates.api_editor.source") }}
      </label>
      <div class="api-field-control">
        <SelectField
          v-model="field.collection"
          :options="sourceOptions"
          :placeholder="
            sourcesLoading
              ? t('shell.common.loading')
              : t('workspace.templates.api_editor.source_placeholder')
          "
          :disabled="sourcesLoading || sourceOptions.length === 0"
        />
        <p
          v-if="!sourcesLoading && sourceOptions.length === 0"
          class="muted small"
        >
          {{ t("workspace.templates.api_editor.no_sources") }}
        </p>
        <p v-if="sourcesError" class="error small">{{ sourcesError }}</p>
      </div>
    </div>

    <!-- Map[] editor -->
    <div class="api-field-row">
      <label class="api-field-label">
        {{ t("workspace.templates.api_editor.columns") }}
      </label>
      <div class="api-field-control">
        <p
          v-if="!field.collection"
          class="muted small"
        >
          {{ t("workspace.templates.api_editor.pick_source_first") }}
        </p>
        <table v-else class="api-map-table">
          <thead>
            <tr>
              <th>{{ t("workspace.templates.api_editor.col.key") }}</th>
              <th>{{ t("workspace.templates.api_editor.col.label") }}</th>
              <th>{{ t("workspace.templates.api_editor.col.type") }}</th>
              <th aria-label="actions"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, idx) in field.map ?? []" :key="idx">
              <td>
                <SelectField
                  v-model="row.key"
                  :options="sourceFieldOptions"
                  :placeholder="t('workspace.templates.api_editor.col.key_placeholder')"
                  :disabled="sourceFieldsLoading"
                />
              </td>
              <td>
                <TextField
                  v-model="row.label"
                  :placeholder="t('workspace.templates.api_editor.col.label_placeholder')"
                />
              </td>
              <td>
                <span class="type-pill" v-if="typeOf(row.key)">
                  {{ typeOf(row.key) }}
                </span>
                <span class="muted small" v-else>—</span>
              </td>
              <td>
                <button
                  type="button"
                  class="tool-btn small"
                  @click="removeRow(idx)"
                  :aria-label="t('common.remove')"
                >
                  ✕
                </button>
              </td>
            </tr>
            <tr v-if="!(field.map ?? []).length">
              <td colspan="4" class="muted small">
                {{ t("workspace.templates.api_editor.no_columns") }}
              </td>
            </tr>
          </tbody>
        </table>
        <button
          v-if="field.collection"
          type="button"
          class="tool-btn small"
          @click="addRow"
        >
          + {{ t("workspace.templates.api_editor.add_column") }}
        </button>
        <p v-if="sourceFieldsError" class="error small">{{ sourceFieldsError }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.api-field-editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.api-field-row {
  display: grid;
  grid-template-columns: 140px 1fr;
  align-items: start;
  gap: 12px;
}

.api-field-label {
  padding-top: 6px;
  font-size: 0.9rem;
  color: var(--form-label, currentColor);
  opacity: 0.85;
}

.api-field-control {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.api-map-table {
  border-collapse: collapse;
  width: 100%;
  font-size: 0.9rem;
}

.api-map-table th,
.api-map-table td {
  padding: 4px 6px;
  border-bottom: 1px solid color-mix(in oklab, currentColor 15%, transparent);
  vertical-align: top;
  text-align: left;
}

.api-map-table th {
  font-weight: 600;
  opacity: 0.85;
}

.type-pill {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  background: color-mix(in oklab, currentColor 12%, transparent);
  font-size: 0.8rem;
}

.tool-btn.small {
  padding: 2px 8px;
  font-size: 0.85rem;
}

.error {
  color: var(--color-danger, #c0392b);
}
</style>
