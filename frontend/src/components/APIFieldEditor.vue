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
import draggable from "vuedraggable";
import { SelectField, TextField } from "./fields";
import {
  Service as TemplateSvc,
  type Field,
  type Template,
  APIMap,
  APIFilter,
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
// Full loaded target template, so the filter row can read field options,
// use_in_statistics, and facets for eligibility + value pickers.
const targetTpl = ref<Template | null>(null);

async function loadSourceFields(filename: string) {
  if (!filename) {
    sourceFields.value = [];
    targetTpl.value = null;
    return;
  }
  sourceFieldsLoading.value = true;
  sourceFieldsError.value = "";
  try {
    const tpl = await TemplateSvc.LoadTemplate(filename);
    targetTpl.value = tpl ?? null;
    sourceFields.value = tpl ? topLevelFields(tpl.fields ?? []) : [];
  } catch (e) {
    sourceFieldsError.value = String((e as Error)?.message ?? e);
    sourceFields.value = [];
    targetTpl.value = null;
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

// ── Filter (one optional predicate, backend-driven values) ─────────────
// Eligible filter fields: facets (filtered via the facet path) + data fields
// that are use_in_statistics (their values are indexed). Other fields can't be
// filtered, so they're not offered; the hint explains why.
const FILTERABLE_TYPES = new Set([
  "text", "dropdown", "radio", "boolean", "number", "range", "date",
]);

type EligibleField = { key: string; label: string; type: string };
const eligibleFilterFields = computed<EligibleField[]>(() => {
  const fields = targetTpl.value?.fields ?? [];
  const out: EligibleField[] = [];
  let depth = 0;
  for (const f of fields) {
    if (f.type === "loopstart") { depth++; continue; }
    if (f.type === "loopstop") { if (depth > 0) depth--; continue; }
    if (depth > 0 || !f.key) continue;
    const eligible = f.type === "facet" || (f.use_in_statistics && FILTERABLE_TYPES.has(f.type));
    if (eligible) out.push({ key: f.key, label: f.label || f.key, type: f.type || "text" });
  }
  return out;
});

const filterFieldOptions = computed(() =>
  eligibleFilterFields.value.map((f) => ({
    value: f.key,
    label: f.label === f.key ? f.key : `${f.label} (${f.key})`,
  })),
);

const filterField = computed(() => props.field.filter?.field_key ?? "");
const filterOp = computed(() => props.field.filter?.op ?? "eq");
const filterValue = computed(() => props.field.filter?.value ?? "");

function filterFieldType(key: string): string {
  return eligibleFilterFields.value.find((f) => f.key === key)?.type ?? "";
}

// Operator sets mirror the backend's apiFilterOps; facets only support eq, and
// numeric/date fields gain the comparisons.
function operatorsFor(type: string): string[] {
  switch (type) {
    case "number":
    case "range":
    case "date":
      return ["eq", "ne", "gt", "ge", "lt", "le"];
    case "facet":
      return ["eq"];
    default:
      return ["eq", "ne"];
  }
}

const FILTER_OP_LABEL_KEYS: Record<string, string> = {
  eq: "workspace.templates.api_editor.filter.op.eq",
  ne: "workspace.templates.api_editor.filter.op.ne",
  gt: "workspace.templates.api_editor.filter.op.gt",
  ge: "workspace.templates.api_editor.filter.op.ge",
  lt: "workspace.templates.api_editor.filter.op.lt",
  le: "workspace.templates.api_editor.filter.op.le",
};

const operatorOptions = computed(() =>
  operatorsFor(filterFieldType(filterField.value)).map((op) => ({
    value: op,
    label: t(FILTER_OP_LABEL_KEYS[op]),
  })),
);

// Value options for the chosen field: dropdown/radio from field.options, facet
// from the facet's option labels, boolean true/false. null = free input.
const filterValueOptions = computed<{ value: string; label: string }[] | null>(() => {
  const key = filterField.value;
  if (!key) return null;
  const fld = (targetTpl.value?.fields ?? []).find((f) => f.key === key);
  if (!fld) return null;
  if (fld.type === "dropdown" || fld.type === "radio") {
    return (fld.options ?? []).map((o) => optionPair(o));
  }
  if (fld.type === "boolean") {
    return [
      { value: "true", label: t("workspace.templates.api_editor.filter.bool_true") },
      { value: "false", label: t("workspace.templates.api_editor.filter.bool_false") },
    ];
  }
  if (fld.type === "facet" && fld.facet_key) {
    const facet = (targetTpl.value?.facets ?? []).find((fc) => fc.key === fld.facet_key);
    return (facet?.options ?? []).map((o) => ({ value: o.label, label: o.label }));
  }
  return null;
});

const filterValueIsDate = computed(
  () => filterFieldType(filterField.value) === "date",
);
const filterValueIsNumber = computed(() => {
  const ty = filterFieldType(filterField.value);
  return ty === "number" || ty === "range";
});

function optionPair(o: unknown): { value: string; label: string } {
  if (o && typeof o === "object") {
    const rec = o as Record<string, unknown>;
    const value = typeof rec.value === "string" ? rec.value : String(rec.value ?? "");
    const label = typeof rec.label === "string" && rec.label.trim() ? rec.label : value;
    return { value, label };
  }
  return { value: String(o), label: String(o) };
}

function ensureFilter(): APIFilter {
  if (!props.field.filter) {
    props.field.filter = APIFilter.createFrom({ field_key: "", op: "eq", value: "" });
  }
  return props.field.filter;
}
function onFilterField(key: string) {
  if (!key) { props.field.filter = null; return; }
  const f = ensureFilter();
  f.field_key = key;
  const ops = operatorsFor(filterFieldType(key));
  if (!ops.includes(f.op)) f.op = ops[0];
  f.value = "";
}
function onFilterOp(op: string) { ensureFilter().op = op; }
function onFilterValue(v: string) { ensureFilter().value = v; }
function clearFilter() { props.field.filter = null; }
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

    <!-- Filter (one optional predicate; narrows the link picker) -->
    <div v-if="field.collection" class="api-field-editor-row">
      <label class="api-field-editor-label">
        {{ t("workspace.templates.api_editor.filter") }}
      </label>
      <div class="api-field-editor-control">
        <p v-if="eligibleFilterFields.length === 0" class="muted small">
          {{ t("workspace.templates.api_editor.filter.none_eligible") }}
        </p>
        <template v-else>
          <div class="api-filter-row">
            <SelectField
              :model-value="filterField"
              :options="filterFieldOptions"
              :placeholder="t('workspace.templates.api_editor.filter.field_placeholder')"
              @update:model-value="(v: string) => onFilterField(v)"
            />
            <SelectField
              v-if="filterField"
              :model-value="filterOp"
              :options="operatorOptions"
              @update:model-value="(v: string) => onFilterOp(v)"
            />
            <SelectField
              v-if="filterField && filterValueOptions"
              :model-value="filterValue"
              :options="filterValueOptions"
              :placeholder="t('workspace.templates.api_editor.filter.value_placeholder')"
              @update:model-value="(v: string) => onFilterValue(v)"
            />
            <input
              v-else-if="filterField && filterValueIsDate"
              type="date"
              class="api-filter-input"
              :value="filterValue"
              @input="(e) => onFilterValue((e.target as HTMLInputElement).value)"
            />
            <input
              v-else-if="filterField && filterValueIsNumber"
              type="number"
              class="api-filter-input"
              :value="filterValue"
              @input="(e) => onFilterValue((e.target as HTMLInputElement).value)"
            />
            <TextField
              v-else-if="filterField"
              :model-value="filterValue"
              :placeholder="t('workspace.templates.api_editor.filter.value_placeholder')"
              @update:model-value="(v: string) => onFilterValue(v)"
            />
            <button
              v-if="filterField"
              type="button"
              class="tool-btn small"
              :aria-label="t('common.remove')"
              @click="clearFilter"
            >
              ✕
            </button>
          </div>
          <p class="muted small">{{ t("workspace.templates.api_editor.filter.hint") }}</p>
        </template>
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
              <th aria-label="drag"></th>
              <th>{{ t("workspace.templates.api_editor.col.key") }}</th>
              <th>{{ t("workspace.templates.api_editor.col.label") }}</th>
              <th>{{ t("workspace.templates.api_editor.col.type") }}</th>
              <th aria-label="actions"></th>
            </tr>
          </thead>
          <draggable
            v-if="(field.map ?? []).length"
            :list="field.map"
            tag="tbody"
            handle=".dnd-handle"
            :animation="150"
            ghost-class="dnd-ghost"
            chosen-class="dnd-chosen"
            drag-class="dnd-drag"
            :item-key="(_e: any, i: number) => i"
          >
            <template #item="{ element: row, index: idx }">
              <tr>
                <td>
                  <span class="dnd-handle" aria-hidden="true">⠿</span>
                </td>
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
            </template>
          </draggable>
          <tbody v-else>
            <tr>
              <td colspan="5" class="muted small">
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
