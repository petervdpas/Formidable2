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
} from "./fields";
import APIFieldEditor from "./APIFieldEditor.vue";
import type { OptionRow } from "./fields/OptionsEditor.vue";
import {
  columnsFor,
  fixedRowsFor,
  lockedColumnsFor,
  SUPPORTED_OPTION_TYPES,
} from "../types/option-presets";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { useToast } from "../composables/useToast";
import { formatError } from "../utils/templateValidation";
import {
  isRowHidden,
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

const expressionItemInvalid = computed<boolean>(() => {
  if (!draft.value) return false;
  const scope = draft.value.level_scope ?? 0;
  return scope > 0 && !!draft.value.expression_item;
});

const canConfirm = computed<boolean>(() => !expressionItemInvalid.value);

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
  (type, prevType) => {
    if (!draft.value) return;
    // Genuine user type change (not the initial load, where prevType is
    // undefined). When the option shape differs from the previous type,
    // stale rows would leak across, so reset; the new type then seeds
    // cleanly (fixed-row types like boolean get their defaults via
    // OptionsEditor). statistics_columns rides along since it names the
    // table's columns.
    if (
      prevType !== undefined &&
      type !== prevType &&
      optionSignature(type ?? "") !== optionSignature(prevType)
    ) {
      draft.value.options = [];
      draft.value.statistics_columns = [];
    }
    if (type === "textarea" && !draft.value.format) {
      draft.value.format = "markdown";
    }
    // A guid field's key is always "id" - mirror backend Normalize
    // (template/normalize.go) so the readonly Key input shows it
    // immediately instead of an empty/stale key.
    if (type === "guid") {
      draft.value.key = "id";
    }
  },
);

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
        </FormRow>

        <FormRow
          v-if="showRow('type')"
          :label="t('workspace.templates.field_edit.row.type')"
        >
          <SelectField v-model="draft.type" :options="typeOptions" />
        </FormRow>

        <FormRow
          v-if="showRow('format')"
          :label="t('workspace.templates.field_edit.row.format')"
        >
          <SelectField
            v-model="draft.format"
            :options="[
              { value: 'markdown', label: 'Markdown' },
              { value: 'plain', label: 'Plain text' },
            ]"
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
          <TextField v-model="defaultAsString" />
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

