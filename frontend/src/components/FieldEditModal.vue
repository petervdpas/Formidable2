<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import {
  FormSection,
  FormRow,
  TextField,
  TextareaField,
  SelectField,
  SwitchField,
  OptionsEditor,
  FieldSelector,
} from "./fields";
import APIFieldEditor from "./APIFieldEditor.vue";
import type { OptionRow } from "./fields/OptionsEditor.vue";
import {
  columnsFor,
  fixedRowsFor,
  lockedColumnsFor,
  SUPPORTED_OPTION_TYPES,
} from "../types/option-presets";
import { Service as TemplateSvc, Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Field, Facet, Formula, ValidationError } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { useToast } from "../composables/useToast";
import { formatError } from "../utils/templateValidation";
import {
  isRowHidden,
  isDataField,
  formulaTargetFieldTypes,
  selectableTypes,
  type FieldEditRowId,
} from "../types/field-types";

const props = defineProps<{
  open: boolean;
  /** The field being edited. Null/undefined opens the modal in
   *  create mode with an empty draft. */
  field: Field | null;
  /** True when adding a new field - surfaces `looper` in the type
   *  dropdown and changes the title to "Add Field". */
  isNew?: boolean;
  /** Optional whitelist of type ids the user is allowed to pick.
   *  Templates pass nothing → every selectable type is offered.
   *  Plugins pass a curated subset (text-shaped + path pickers)
   *  so workflow-irrelevant types (image, list, api, …) are
   *  hidden from the dropdown. */
  allowedTypes?: string[];
  /** Facets declared on the surrounding template - used to populate
   *  the facet_key binding dropdown when the user picks type "facet".
   *  Empty (the default) means "no facets configured": the facet rows
   *  still show but the binding picker is empty and Confirm is
   *  disabled with a hint pointing the user at the Facets tab. */
  availableFacets?: Facet[];
  /** Formulas declared on the surrounding template draft - populate the
   *  source picker when the user picks type "formula". Empty means "no
   *  formulas configured": the picker is empty and Confirm is disabled
   *  with a hint pointing the user at the Formulas tab. */
  availableFormulas?: Formula[];
  /** The surrounding template's fields - the candidate write targets for
   *  a formula field (filtered to real data fields, excluding the formula
   *  field itself). */
  availableFields?: Field[];
  /** Loop summary-field candidates (loopstart only): the loop's direct
   *  child fields as {value,label} pairs, supplied by the parent from
   *  the backend. Feeds the Summary field picker so a loopstart can bind
   *  its collapsed-item summary to one of its own children. Empty for
   *  every non-loop field type. */
  summaryFieldOptions?: { key: string; label: string }[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "confirm", field: Field): void;
}>();

const { t } = useI18n();
const toast = useToast();

// Local working copy. We don't mutate props.field directly - only
// commit changes when the user clicks Confirm.
const draft = ref<Field | null>(null);

const isFacetType = computed(() => draft.value?.type === "facet");
const isFormulaType = computed(() => draft.value?.type === "formula");

// ── Formula field bindings (source formula + target data field + trigger) ──
const formulaSourceOptions = computed(() =>
  (props.availableFormulas ?? []).map((f) => ({
    value: f.key,
    label: f.label ? `${f.label} (${f.key})` : f.key,
  })),
);

// The selected source formula's result type scopes which fields can be targets
// (a text formula can't write into a number field, etc.). Default "number"
// mirrors the backend (a blank formula type normalises to number).
const selectedFormulaType = computed<string>(() => {
  const key = (draft.value?.formula_key ?? "").trim();
  const f = (props.availableFormulas ?? []).find((x) => x.key === key);
  return f?.type || "number";
});

// Targets are root (level 0) data fields whose type can hold the formula's
// result, excluding the formula field itself. Looped fields are excluded: the
// engine evaluates whole-form context, so a per-iteration target can't work.
// Built from the live props so renames + formula-type changes flow through.
const formulaTargetOptions = computed(() => {
  const accepted = formulaTargetFieldTypes(selectedFormulaType.value);
  return (props.availableFields ?? [])
    .filter(
      (f) =>
        f.key &&
        f.key !== draft.value?.key &&
        (f.level_scope ?? 0) === 0 &&
        isDataField(f.type) &&
        accepted.includes(f.type),
    )
    .map((f) => ({ value: f.key, label: f.label ? `${f.label} (${f.key})` : f.key }));
});

const formulaTriggerOptions = computed(() => [
  { value: "save", label: t("workspace.templates.field_edit.formula.trigger_save") },
  { value: "load", label: t("workspace.templates.field_edit.formula.trigger_load") },
  { value: "live", label: t("workspace.templates.field_edit.formula.trigger_live") },
]);

const formulaTriggerValue = computed<string>({
  get: () => draft.value?.trigger || "save",
  set: (v: string) => {
    if (draft.value) draft.value.trigger = v;
  },
});

// Changing the source formula can change its result type, which may make the
// picked target incompatible. Clear it so the picker doesn't carry a stale,
// now-invalid binding through Confirm (backend validation would reject it too).
watch(
  () => draft.value?.formula_key,
  () => {
    if (!isFormulaType.value || !draft.value) return;
    const cur = (draft.value.target_key ?? "").trim();
    if (cur === "") return;
    if (!formulaTargetOptions.value.some((o) => o.value === cur)) {
      draft.value.target_key = "";
    }
  },
);

// Summary-field picker (loopstart only). Candidates come from the
// backend via the parent; FieldSelector renders the list and the
// leading "(none)" entry. The Field carries summary_field as an
// optional string, so the model coerces null/undefined to "".
const summaryFieldValue = computed<string>({
  get: () => draft.value?.summary_field ?? "",
  set: (v: string) => {
    if (draft.value) draft.value.summary_field = v;
  },
});

const facetBindingMissing = computed<boolean>(() => {
  if (!isFacetType.value) return false;
  const key = (draft.value?.facet_key ?? "").trim();
  if (key === "") return true;
  const known = (props.availableFacets ?? []).some((f) => f.key === key);
  return !known;
});

// Every field needs a key (its identifier / data slot). guid auto-keys to "id"
// so it's never blocked. Without this a keyless field could be confirmed and
// then never be addressable (e.g. a formula field's Compute button couldn't
// find it).
const keyMissing = computed<boolean>(() => (draft.value?.key ?? "").trim() === "");

// Confirm is gated on the backend: the candidate field is validated against the
// surrounding template + schema (duplicate/missing keys, bindings, type/level
// rules) and may only be confirmed when that returns no errors. The frontend
// owns no validation rules of its own; the inline hints below are presentation.
const fieldErrors = ref<ValidationError[]>([]);
const validating = ref(false);
// A pristine new field shouldn't shout "missing key" before the user types
// anything. Errors surface once the draft is touched (or always when editing an
// existing field, where pre-existing problems are worth showing immediately).
const touched = ref(false);
let skipNextTouch = false;
let validateTimer: ReturnType<typeof setTimeout> | null = null;

function candidateTemplate(): Template {
  return new Template({
    fields: props.availableFields ?? [],
    facets: props.availableFacets ?? [],
    formulas: props.availableFormulas ?? [],
  });
}

async function runValidation(): Promise<void> {
  if (!draft.value) {
    fieldErrors.value = [];
    return;
  }
  validating.value = true;
  try {
    const originalKey = props.isNew ? "" : (props.field?.key ?? "");
    fieldErrors.value =
      (await TemplateSvc.ValidateField(
        candidateTemplate(),
        draft.value,
        originalKey,
        !!props.isNew,
      )) ?? [];
  } catch {
    // A transport hiccup shouldn't hard-block the editor; surface nothing and
    // let the real save re-validate authoritatively.
    fieldErrors.value = [];
  } finally {
    validating.value = false;
  }
}

watch(
  () => draft.value,
  () => {
    if (skipNextTouch) {
      skipNextTouch = false;
    } else {
      touched.value = true;
    }
    if (validateTimer) clearTimeout(validateTimer);
    validateTimer = setTimeout(() => void runValidation(), 200);
  },
  { deep: true },
);

const fieldErrorMessages = computed<string[]>(() =>
  fieldErrors.value.map((e) => {
    const f = formatError(e);
    return t(f.key, f.args);
  }),
);

// Show the panel only once the field has been touched (or when editing an
// existing field); never on a freshly-opened blank new field.
const showErrors = computed<boolean>(
  () => fieldErrorMessages.value.length > 0 && (!props.isNew || touched.value),
);

const canConfirm = computed<boolean>(
  () => !validating.value && fieldErrors.value.length === 0,
);

const facetBindingOptions = computed(() =>
  (props.availableFacets ?? []).map((f) => ({ value: f.key, label: f.key })),
);

const facetFormatOptions = computed(() => [
  { value: "radio", label: t("workspace.templates.field_edit.facet.presentation_radio") },
  { value: "dropdown", label: t("workspace.templates.field_edit.facet.presentation_dropdown") },
]);

const textareaFormatOptions = computed(() => [
  { value: "markdown", label: "Markdown" },
  { value: "plain", label: "Plain text" },
]);

const formatOptionsForType = computed(() => {
  if (isFacetType.value) return facetFormatOptions.value;
  return textareaFormatOptions.value;
});

// Default for a facet field is one of the bound facet's option labels
// (or unset). Picker options come from the live availableFacets prop
// so renaming a facet's options updates the picker immediately.
const boundFacet = computed(() => {
  if (!isFacetType.value) return null;
  const key = (draft.value?.facet_key ?? "").trim();
  if (key === "") return null;
  return (props.availableFacets ?? []).find((f) => f.key === key) ?? null;
});

const facetDefaultOptions = computed(() => {
  const opts = boundFacet.value?.options ?? [];
  // Lead with a selectable "(not set)" entry so the user can clear a
  // previously-picked default. SelectField's placeholder prop renders
  // the same label but as a disabled option, which traps the user
  // once any real value has been picked.
  return [
    { value: "", label: t("facet.field.placeholder") },
    ...opts.map((o) => ({ value: o.label, label: o.label })),
  ];
});

const facetDefaultValue = computed<string>({
  get: () => {
    const v = draft.value?.default;
    return typeof v === "string" ? v : "";
  },
  set: (v: string) => {
    if (!draft.value) return;
    // Empty selection clears Default to null, matching backend Normalize
    // (template/normalize.go normalizeFacetFieldDefaults clears empty
    // strings and unknown labels to nil).
    draft.value.default = v === "" ? null : v;
  },
});

// A bound facet field must declare a default (the backend rejects an empty
// one): forms can never auto-fill a defaultless facet. Block Confirm until the
// author picks one. Skipped while the binding itself is unresolved.
const facetDefaultMissing = computed<boolean>(() => {
  if (!isFacetType.value) return false;
  if (facetBindingMissing.value) return false;
  return facetDefaultValue.value.trim() === "";
});

// Changing the bound facet invalidates any previously-picked Default
// (its labels may no longer exist). Clear it so the picker doesn't
// silently carry a stale value through Confirm; backend Normalize
// would clear it on save anyway, this just keeps the UI honest.
watch(
  () => draft.value?.facet_key,
  () => {
    if (!isFacetType.value || !draft.value) return;
    const cur = typeof draft.value.default === "string" ? draft.value.default : "";
    if (cur === "") return;
    const known = facetDefaultOptions.value.some((o) => o.value === cur);
    if (!known) draft.value.default = null;
  },
);

watch(
  () => draft.value?.expression_item,
  (now, prev) => {
    if (!draft.value) return;
    if (!now || prev) return;
    const scope = draft.value.level_scope ?? 0;
    if (scope === 0) return;
    const formatted = formatError({
      type: "expression-item-non-root",
      key: draft.value.key,
      detail: { levelScope: scope },
    } as never);
    toast.error(formatted.key, formatted.args);
  },
);

function emptyDraft(): Field {
  // Sensible starting shape - text type, blank key/label.
  return {
    key: "",
    type: "text",
    label: "",
  } as Field;
}

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    if (props.field) {
      // Deep-copy so cancelling discards cleanly. Backend Normalize
      // (template.normalizeStatisticsColumns) is the source of truth
      // for deduping/validating statistics_columns; the per-row picker
      // below only prevents creating duplicates as a UX accelerator.
      draft.value = JSON.parse(JSON.stringify(props.field));
    } else {
      draft.value = emptyDraft();
    }
    // Validate the freshly-opened draft up front so Confirm starts disabled
    // until the backend confirms the field is clean. The draft assignment above
    // trips the deep watch once; skip that so a pristine new field isn't marked
    // touched (and its errors stay hidden until the user actually edits).
    touched.value = false;
    skipNextTouch = true;
    validating.value = true;
    void runValidation();
  },
  { immediate: true },
);

const typeOptions = computed(() => {
  if (!draft.value) return [];
  let types = selectableTypes(draft.value.type || "text", props.isNew);
  if (props.allowedTypes && props.allowedTypes.length > 0) {
    const allow = new Set(props.allowedTypes);
    // Always keep the current type even if it's outside the
    // whitelist - switching away from it is fine, but we shouldn't
    // hide the row's own type label while it's still selected.
    types = types.filter(
      (t) => allow.has(t.id) || t.id === draft.value?.type,
    );
  }
  return types.map((td) => ({
    value: td.id,
    label: t(td.labelKey),
  }));
});

// Type-driven defaults. When the user (or the initial seed) lands on
// textarea, Format should be "markdown" - that's what the dropdown
// shows by default, and it's what the original Formidable saves to
// YAML. Without this, an empty draft confirms with format unset.
// Shape signature for a type's options: the ordered column keys plus the
// fixed-row count. Two types with the same signature (dropdown / radio /
// multioption all share value+label, no fixed rows) can carry options
// across a type switch; a differing signature means the old options don't
// fit the new editor (boolean's fixed true/false rows leaking into range
// or table, list's type column, etc.) and must be reset.
function optionSignature(typeId: string): string {
  const cols = (columnsFor(typeId) ?? []).map((c) => c.key).join(",");
  const fixed = (fixedRowsFor(typeId) ?? []).length;
  return `${cols}|${fixed}`;
}

watch(
  () => draft.value?.type,
  (type) => {
    if (!draft.value) return;
    if (type === "textarea" && !draft.value.format) {
      draft.value.format = "markdown";
    }
    if (type === "facet" && !draft.value.format) {
      draft.value.format = "radio";
    }
    if (type === "formula" && !draft.value.trigger) {
      draft.value.trigger = "save";
    }
    // A guid field's key is always "id" - mirror backend Normalize
    // (template/normalize.go) so the readonly Key input shows it
    // immediately instead of an empty/stale key.
    if (type === "guid") {
      draft.value.key = "id";
    }
  },
);

// Option-reset belongs to a USER type change, never to loading a field.
// When the new type's option shape differs from the old one's, stale rows
// would leak across, so reset; the new type then seeds cleanly. Driving
// this from the dropdown (not a watch on draft.type) avoids wiping a
// freshly-loaded field's options just because the previous draft in the
// session had a different type.
function onTypeChange(next: string) {
  if (!draft.value) return;
  const prev = draft.value.type || "";
  if (next !== prev && optionSignature(next) !== optionSignature(prev)) {
    draft.value.options = [];
    draft.value.statistics_columns = [];
  }
  // Clear facet_key when leaving facet (Normalize would strip it on
  // save anyway, but the UI shouldn't carry a stale binding while the
  // new type's editor sections are visible). When entering facet,
  // reset format to the canonical "radio" if currently empty/unknown.
  if (prev === "facet" && next !== "facet") {
    draft.value.facet_key = "";
  }
  if (next === "facet") {
    if (!draft.value.facet_key) draft.value.facet_key = "";
    if (draft.value.format !== "radio" && draft.value.format !== "dropdown") {
      draft.value.format = "radio";
    }
  }
  // Clear formula bindings when leaving formula; seed a trigger when entering
  // (Normalize defaults to "save", but the UI shouldn't carry stale bindings).
  if (prev === "formula" && next !== "formula") {
    draft.value.formula_key = "";
    draft.value.target_key = "";
    draft.value.trigger = "";
  }
  if (next === "formula" && !draft.value.trigger) {
    draft.value.trigger = "save";
  }
  draft.value.type = next;
}

function showRow(row: FieldEditRowId): boolean {
  if (!draft.value) return false;
  return !isRowHidden(draft.value.type || "text", row);
}

// Default value editor - type-aware. For string types, plain input.
// For booleans, the "default" doesn't really exist (omit). For lists/
// tables/tags it's a textarea (CSV / JSON, free-form for now).
// Field.collapsible is nullable (*bool on the Go side, omitempty),
// so SwitchField's `boolean` model needs a coercing wrapper.
const collapsibleBool = computed<boolean>({
  get: () => draft.value?.collapsible === true,
  set: (v: boolean) => {
    if (!draft.value) return;
    draft.value.collapsible = v;
  },
});

const defaultAsString = computed({
  get: () => {
    const v = draft.value?.default;
    if (v == null) return "";
    if (typeof v === "string") return v;
    if (Array.isArray(v)) return v.join(", ");
    return String(v);
  },
  set: (v: string) => {
    if (!draft.value) return;
    // Backend `template.Normalize` (called on SaveTemplate) is the
    // authoritative type-coercion pass - number/range string → float,
    // boolean string → bool, tags/multioption/list string → array.
    // Mirroring it here would be duplication and a source of drift,
    // so the modal just stores the raw text; the round-trip restores
    // the typed value next load.
    draft.value.default = v;
  },
});

// Options - per-type column structure (boolean uses [value,label],
// list uses [type,value,label], table uses [key,type,label], etc.).
// Types not in the supported set get a "not available" message.
const optionsSupported = computed(() => SUPPORTED_OPTION_TYPES.has(draft.value?.type || ""));

const optionColumns = computed(() => columnsFor(draft.value?.type || "") ?? []);
const optionFixedRows = computed(() => fixedRowsFor(draft.value?.type || "") ?? undefined);
const optionLockedColumns = computed(() => lockedColumnsFor(draft.value?.type || ""));

const optionRows = computed<OptionRow[]>({
  get: () => {
    const opts = draft.value?.options ?? [];
    if (!Array.isArray(opts)) return [];
    // Normalize each entry into a row object. Strings become {value, label}
    // pairs (single-column would lose label, so default to value=label).
    return opts.map((o) => {
      if (o && typeof o === "object" && !Array.isArray(o)) {
        return { ...(o as Record<string, unknown>) };
      }
      const s = String(o);
      return { value: s, label: s };
    });
  },
  set: (rows) => {
    if (!draft.value) return;
    draft.value.options = rows;
  },
});

// Per-column statistics selection (table only). statistics_columns
// stores the selected column `value` keys; the picker mirrors the
// Options editor (rows added/removed with +/−). Every table column is
// eligible - string columns included - so any column can feed a
// distribution. Lists carry a single column, so the field-level toggle
// covers them.
interface StatColumn {
  value: string;
  label: string;
}
const eligibleStatColumns = computed<StatColumn[]>(() => {
  if (draft.value?.type !== "table") return [];
  const opts = draft.value.options ?? [];
  if (!Array.isArray(opts)) return [];
  return opts
    .map((o) => {
      if (o && typeof o === "object" && !Array.isArray(o)) {
        const r = o as Record<string, unknown>;
        const value = String(r.value ?? "");
        return { value, label: String(r.label ?? value) };
      }
      const s = String(o);
      return { value: s, label: s };
    })
    .filter((c) => c.value !== "");
});

const selectedStatColumns = computed<string[]>(() =>
  Array.isArray(draft.value?.statistics_columns) ? draft.value!.statistics_columns! : [],
);

// Each column may be selected at most once. A row's dropdown offers its
// own current value plus any column not already chosen by another row,
// so duplicates can't be picked.
function statColumnOptionsFor(idx: number) {
  const selected = selectedStatColumns.value;
  const own = selected[idx];
  const usedElsewhere = new Set(selected.filter((_, i) => i !== idx));
  return eligibleStatColumns.value
    .filter((c) => c.value === own || !usedElsewhere.has(c.value))
    .map((c) => ({ value: c.value, label: c.label }));
}

// + is offered only while there's still an unselected column.
const canAddStatColumn = computed(
  () => selectedStatColumns.value.length < eligibleStatColumns.value.length,
);

function setStatColumnAt(idx: number, value: string): void {
  if (!draft.value) return;
  const cur = selectedStatColumns.value.slice();
  cur[idx] = value;
  draft.value.statistics_columns = cur;
}

function removeStatColumnAt(idx: number): void {
  if (!draft.value) return;
  draft.value.statistics_columns = selectedStatColumns.value.filter((_, i) => i !== idx);
}

function addStatColumn(): void {
  if (!draft.value) return;
  const cur = selectedStatColumns.value;
  const next = eligibleStatColumns.value.find((c) => !cur.includes(c.value));
  if (!next) return; // every eligible column already selected
  draft.value.statistics_columns = [...cur, next.value];
}

function submit() {
  if (!draft.value) return;
  if (!canConfirm.value) return;
  emit("confirm", draft.value);
}

// The loop pair (loopstart / loopstop) shares the single `looper` color
// set, matching the field-row list. The raw type still labels the pill;
// only the color-var lookup is canonicalised so the dialog isn't left on
// the default dark floor.
function colorTypeFor(type: string): string {
  if (type === "loopstart" || type === "loopstop") return "looper";
  return type;
}

// Badge in the modal header - uses the per-type badge color so it
// pops on the type-tinted dialog floor.
const typePillStyle = computed(() => {
  const type = colorTypeFor(draft.value?.type || "text");
  return {
    background: `var(--field-type-${type}-badge, var(--color-accent))`,
    color: `var(--field-type-${type}-text, #fff)`,
  };
});

// Per-type dialog tint - the modal's full background takes the field
// type color, matching the original Formidable UX. Form labels +
// borders pick the right contrast via .modal-dialog.tinted overrides
// in styles/field-types.css.
const dialogStyle = computed<Record<string, string>>(() => {
  const type = colorTypeFor(draft.value?.type || "text");
  return {
    "--type-bg":     `var(--field-type-${type}-bg, var(--color-bg))`,
    "--type-text":   `var(--field-type-${type}-text, var(--color-text))`,
    "--type-border": `var(--field-type-${type}-border, var(--color-border))`,
  };
});
</script>

<template>
  <Modal
    :open="open"
    width="640px"
    dialog-class="field-edit-tinted"
    :dialog-style="dialogStyle"
    scroll
    @close="emit('close')"
  >
    <template #title>
      <span>{{ isNew
          ? t('workspace.templates.field_edit.title_create')
          : t('workspace.templates.field_edit.title') }}</span>
      <span
        v-if="draft"
        class="field-type-pill"
        :style="typePillStyle"
      >({{ (draft.type || '').toUpperCase() }})</span>
    </template>

    <div v-if="draft" class="field-edit">
      <FormSection>
        <FormRow
          v-if="showRow('key')"
          :label="t('workspace.templates.field_edit.row.key')"
        >
          <TextField
            v-model="draft.key"
            :readonly="draft.type === 'guid'"
            placeholder="snake_case_key"
          />
          <p v-if="keyMissing" class="muted small">
            {{ t('workspace.templates.field_edit.key_required') }}
          </p>
        </FormRow>

        <FormRow
          v-if="showRow('type')"
          :label="t('workspace.templates.field_edit.row.type')"
        >
          <SelectField
            :model-value="draft.type"
            :options="typeOptions"
            @update:model-value="onTypeChange"
          />
        </FormRow>

        <FormRow
          v-if="showRow('summary_field')"
          :label="t('workspace.templates.field_edit.row.summary_field')"
        >
          <FieldSelector
            v-model="summaryFieldValue"
            :fields="summaryFieldOptions ?? []"
            :empty-label="t('workspace.templates.field_edit.row.summary_field_none')"
          />
        </FormRow>

        <FormRow
          v-if="showRow('expression_item')"
          :label="t('workspace.templates.field_edit.row.expression_item')"
        >
          <SwitchField
            v-model="draft.expression_item"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </FormRow>

        <FormRow
          v-if="showRow('two_column')"
          :label="t('workspace.templates.field_edit.row.two_column')"
        >
          <SwitchField
            v-model="draft.two_column"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </FormRow>

        <FormRow
          v-if="showRow('collapsible')"
          :label="t('workspace.templates.field_edit.row.collapsible')"
        >
          <SwitchField
            v-model="collapsibleBool"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </FormRow>

        <FormRow
          v-if="showRow('readonly')"
          :label="t('workspace.templates.field_edit.row.readonly')"
        >
          <SwitchField
            v-model="draft.readonly"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </FormRow>

        <FormRow
          v-if="showRow('use_in_statistics')"
          :label="t('workspace.templates.field_edit.row.use_in_statistics')"
        >
          <SwitchField
            v-model="draft.use_in_statistics"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </FormRow>

        <FormRow
          v-if="showRow('format')"
          :label="isFacetType
            ? t('workspace.templates.field_edit.facet.presentation_label')
            : t('workspace.templates.field_edit.row.format')"
        >
          <SelectField
            v-model="draft.format"
            :options="formatOptionsForType"
          />
        </FormRow>

        <FormRow
          v-if="isFacetType"
          :label="t('workspace.templates.field_edit.facet.binding_label')"
        >
          <p
            v-if="(availableFacets ?? []).length === 0"
            class="muted small"
          >
            {{ t('workspace.templates.field_edit.facet.binding_empty_hint') }}
          </p>
          <SelectField
            v-else
            v-model="draft.facet_key"
            :options="facetBindingOptions"
            :placeholder="t('workspace.templates.field_edit.facet.binding_placeholder')"
          />
        </FormRow>

        <template v-if="isFormulaType">
          <FormRow :label="t('workspace.templates.field_edit.formula.source_label')">
            <p
              v-if="formulaSourceOptions.length === 0"
              class="muted small"
            >
              {{ t('workspace.templates.field_edit.formula.source_empty_hint') }}
            </p>
            <SelectField
              v-else
              v-model="draft.formula_key"
              :options="formulaSourceOptions"
              :placeholder="t('workspace.templates.field_edit.formula.source_placeholder')"
            />
          </FormRow>

          <FormRow :label="t('workspace.templates.field_edit.formula.target_label')">
            <p
              v-if="formulaTargetOptions.length === 0"
              class="muted small"
            >
              {{ t('workspace.templates.field_edit.formula.target_empty_hint') }}
            </p>
            <SelectField
              v-else
              v-model="draft.target_key"
              :options="formulaTargetOptions"
              :placeholder="t('workspace.templates.field_edit.formula.target_placeholder')"
            />
          </FormRow>

          <FormRow
            :label="t('workspace.templates.field_edit.formula.trigger_label')"
            :description="t('workspace.templates.field_edit.formula.trigger_hint')"
          >
            <SelectField
              v-model="formulaTriggerValue"
              :options="formulaTriggerOptions"
            />
          </FormRow>
        </template>

        <FormRow
          v-if="showRow('label')"
          :label="t('workspace.templates.field_edit.row.label')"
        >
          <TextField v-model="draft.label" />
        </FormRow>

        <FormRow
          v-if="showRow('description')"
          :label="t('workspace.templates.field_edit.row.description')"
        >
          <TextareaField v-model="draft.description" :rows="2" />
        </FormRow>

        <FormRow
          v-if="showRow('default')"
          :label="t('workspace.templates.field_edit.row.default')"
        >
          <template v-if="isFacetType">
            <SelectField
              v-model="facetDefaultValue"
              :options="facetDefaultOptions"
            />
            <p v-if="facetDefaultMissing" class="muted small">
              {{ t('workspace.templates.field_edit.facet.default_required') }}
            </p>
          </template>
          <TextField v-else v-model="defaultAsString" />
        </FormRow>

        <FormRow
          v-if="showRow('options')"
          :label="t('workspace.templates.field_edit.row.options')"
        >
          <OptionsEditor
            v-if="optionsSupported"
            v-model="optionRows"
            :columns="optionColumns"
            :fixed-rows="optionFixedRows"
            :locked-columns="optionLockedColumns"
          />
          <p v-else class="muted small options-unavailable">
            {{ t('workspace.templates.field_edit.row.options_unavailable') }}
          </p>
        </FormRow>

        <FormRow
          v-if="draft.type === 'table' && draft.use_in_statistics"
          :label="t('workspace.templates.field_edit.row.statistics_columns')"
        >
          <p v-if="eligibleStatColumns.length === 0" class="muted small">
            {{ t('workspace.templates.field_edit.row.statistics_columns_empty') }}
          </p>
          <div v-else class="options-editor">
            <div class="options-rows">
              <div
                v-for="(colKey, i) in selectedStatColumns"
                :key="i"
                class="options-row"
              >
                <SelectField
                  :model-value="colKey"
                  :options="statColumnOptionsFor(i)"
                  class="options-cell"
                  @update:model-value="(v: string) => setStatColumnAt(i, v)"
                />
                <button
                  type="button"
                  class="btn-ghost-icon"
                  :title="t('workspace.templates.options.remove_choice')"
                  @click="removeStatColumnAt(i)"
                >−</button>
              </div>
            </div>
            <button
              v-if="canAddStatColumn"
              type="button"
              class="btn-ghost-block"
              :title="t('workspace.templates.field_edit.row.statistics_columns_add')"
              @click="addStatColumn"
            >+</button>
          </div>
        </FormRow>
      </FormSection>

      <FormSection
        v-if="draft.type === 'api'"
        :title="t('workspace.templates.api_editor.section')"
      >
        <APIFieldEditor :field="draft" />
      </FormSection>

      <details v-if="showErrors" class="field-edit-errors">
        <summary>
          {{ t('workspace.templates.field_edit.errors_summary', [fieldErrorMessages.length]) }}
        </summary>
        <ul>
          <li v-for="(msg, i) in fieldErrorMessages" :key="i">{{ msg }}</li>
        </ul>
      </details>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canConfirm"
        @click="submit"
      >
        {{ t('workspace.templates.field_edit.confirm') }}
      </button>
    </template>
  </Modal>
</template>

