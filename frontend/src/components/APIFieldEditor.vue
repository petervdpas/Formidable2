<script setup lang="ts">
// Editor for the api (relation reference) field's config:
//   • collection - the TARGET template (filename) the field references.
//                  Restricted to templates the host has a DECLARED relation
//                  to (the relation must pre-exist, declared in the Relations
//                  tab); the field operates within the relations set.
//   • map        - the subset of target fields to edit + display inline.
//                  Type is read-only (resolved live from the target template).
//
// Cardinality is NOT set here: it is read-only, derived from the declared
// relation. The backend owns the value->label map and the single/multi rule
// (CardinalityOption.source_many).

import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { SelectField, TextField } from "./fields";
import {
  Service as TemplateSvc,
  type Field,
  APIMap,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as DataproviderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";
import {
  Service as RelationSvc,
  type Relation,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation";
import type { CardinalityOption } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation/models";

const props = defineProps<{
  /** Bound field draft. We mutate `collection` and `map` directly so
   *  FieldEditModal's deep-copy-on-open / commit-on-confirm cycle
   *  Just Works without per-attribute emits. */
  field: Field;
  /** The host template's filename, so the target dropdown can be scoped to
   *  its declared relations. Empty disables the dropdown (no relations to
   *  pick from). */
  hostTemplate: string;
}>();

const { t } = useI18n();

// ── Declared relations of the host template (the allowed targets) ──────
const relations = ref<Relation[]>([]);
const relationsLoading = ref(false);
const relationsError = ref("");
const templateNames = ref<Record<string, string>>({});

async function loadRelations() {
  if (!props.hostTemplate) {
    relations.value = [];
    return;
  }
  relationsLoading.value = true;
  relationsError.value = "";
  try {
    relations.value = (await RelationSvc.GetRelations(props.hostTemplate)) ?? [];
    const tpls = (await DataproviderSvc.ListCollectionTemplates()) ?? [];
    const names: Record<string, string> = {};
    for (const s of tpls) names[s.filename] = s.name || s.stem || s.filename;
    templateNames.value = names;
  } catch (e) {
    relationsError.value = String((e as Error)?.message ?? e);
    relations.value = [];
  } finally {
    relationsLoading.value = false;
  }
}

watch(() => props.hostTemplate, () => void loadRelations(), { immediate: true });

const targetOptions = computed(() =>
  relations.value.map((r) => ({
    value: r.to,
    label: templateNames.value[r.to] || r.to,
  })),
);

// ── Cardinality (read-only, derived from the declared relation) ────────
const cardinalityChoices = ref<CardinalityOption[]>([]);
void RelationSvc.Cardinalities().then((o) => (cardinalityChoices.value = o ?? []));

const selectedRelation = computed<Relation | null>(
  () => relations.value.find((r) => r.to === props.field.collection) ?? null,
);

const cardinalityLabel = computed(() => {
  const rel = selectedRelation.value;
  if (!rel) return "";
  const opt = cardinalityChoices.value.find((o) => o.value === rel.cardinality);
  return opt ? t(opt.label_key) : String(rel.cardinality);
});

// ── Target template's level-0 field roster (the Map subset source) ─────
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
    sourceFields.value = tpl ? topLevelFields(tpl.fields ?? []) : [];
  } catch (e) {
    sourceFieldsError.value = String((e as Error)?.message ?? e);
    sourceFields.value = [];
  } finally {
    sourceFieldsLoading.value = false;
  }
}

watch(
  () => props.field.collection,
  (collection) => void loadSourceFields(collection ?? ""),
  { immediate: true },
);

// Walk the roster, skipping loop bodies and marker/automatic types.
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
    out.push({ key: f.key, label: f.label || f.key, type: f.type || "text" });
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

// ── Map[] mutators (direct on the draft, stable array reference) ───────
function ensureMap(): APIMap[] {
  if (!Array.isArray(props.field.map)) props.field.map = [];
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
    <!-- Target template (declared relations only) -->
    <div class="api-field-editor-row">
      <label class="api-field-editor-label">
        {{ t("workspace.templates.api_editor.target") }}
      </label>
      <div class="api-field-editor-control">
        <SelectField
          v-model="field.collection"
          :options="targetOptions"
          :placeholder="
            relationsLoading
              ? t('shell.common.loading')
              : t('workspace.templates.api_editor.target_placeholder')
          "
          :disabled="relationsLoading || targetOptions.length === 0"
        />
        <p
          v-if="!relationsLoading && targetOptions.length === 0"
          class="muted small"
        >
          {{ t("workspace.templates.api_editor.no_relations") }}
        </p>
        <p v-if="relationsError" class="error small">{{ relationsError }}</p>
      </div>
    </div>

    <!-- Cardinality (read-only, from the declared relation) -->
    <div v-if="field.collection" class="api-field-editor-row">
      <label class="api-field-editor-label">
        {{ t("workspace.templates.api_editor.cardinality") }}
      </label>
      <div class="api-field-editor-control">
        <span v-if="cardinalityLabel" class="api-cardinality-pill">
          {{ cardinalityLabel }}
        </span>
        <span v-else class="muted small">-</span>
      </div>
    </div>

    <!-- Map[] editor (the editable + displayed subset of target fields) -->
    <div class="api-field-editor-row">
      <label class="api-field-editor-label">
        {{ t("workspace.templates.api_editor.columns") }}
      </label>
      <div class="api-field-editor-control">
        <p v-if="!field.collection" class="muted small">
          {{ t("workspace.templates.api_editor.pick_target_first") }}
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
                <span class="type-pill" v-if="typeOf(row.key)">{{ typeOf(row.key) }}</span>
                <span class="muted small" v-else>-</span>
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
