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
import type { OptionRow } from "./fields/OptionsEditor.vue";
import { columnsFor, SUPPORTED_OPTION_TYPES } from "../types/option-presets";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
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
  /** True when adding a new field — surfaces `looper` in the type
   *  dropdown and changes the title to "Add Field". */
  isNew?: boolean;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "confirm", field: Field): void;
}>();

const { t } = useI18n();

// Local working copy. We don't mutate props.field directly — only
// commit changes when the user clicks Confirm.
const draft = ref<Field | null>(null);

function emptyDraft(): Field {
  // Sensible starting shape — text type, blank key/label.
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
      // Deep-copy so cancelling discards cleanly.
      draft.value = JSON.parse(JSON.stringify(props.field));
    } else {
      draft.value = emptyDraft();
    }
  },
  { immediate: true },
);

const typeOptions = computed(() => {
  if (!draft.value) return [];
  return selectableTypes(draft.value.type || "text", props.isNew).map((td) => ({
    value: td.id,
    label: t(td.labelKey),
  }));
});

// Type-driven defaults. When the user (or the initial seed) lands on
// textarea, Format should be "markdown" — that's what the dropdown
// shows by default, and it's what the original Formidable saves to
// YAML. Without this, an empty draft confirms with format unset.
watch(
  () => draft.value?.type,
  (type) => {
    if (!draft.value) return;
    if (type === "textarea" && !draft.value.format) {
      draft.value.format = "markdown";
    }
  },
);

function showRow(row: FieldEditRowId): boolean {
  if (!draft.value) return false;
  return !isRowHidden(draft.value.type || "text", row);
}

// Default value editor — type-aware. For string types, plain input.
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
    draft.value.default = v;
  },
});

// Options — per-type column structure (boolean uses [value,label],
// list uses [type,value,label], table uses [key,type,label], etc.).
// Types not in the supported set get a "not available" message.
const optionsSupported = computed(() => SUPPORTED_OPTION_TYPES.has(draft.value?.type || ""));

const optionColumns = computed(() => columnsFor(draft.value?.type || "") ?? []);

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

function submit() {
  if (!draft.value) return;
  emit("confirm", draft.value);
}

// Badge in the modal header — uses the per-type badge color so it
// pops on the type-tinted dialog floor.
const typePillStyle = computed(() => {
  const type = draft.value?.type || "text";
  return {
    background: `var(--field-type-${type}-badge, var(--color-accent))`,
    color: `var(--field-type-${type}-text, #fff)`,
  };
});

// Per-type dialog tint — the modal's full background takes the field
// type color, matching the original Formidable UX. Form labels +
// borders pick the right contrast via .modal-dialog.tinted overrides
// in styles/field-types.css.
const dialogStyle = computed<Record<string, string>>(() => {
  const type = draft.value?.type || "text";
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
          />
          <p v-else class="muted small options-unavailable">
            {{ t('workspace.templates.field_edit.row.options_unavailable') }}
          </p>
        </FormRow>
      </FormSection>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="submit">
        {{ t('workspace.templates.field_edit.confirm') }}
      </button>
    </template>
  </Modal>
</template>

