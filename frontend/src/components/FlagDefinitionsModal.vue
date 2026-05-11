<script setup lang="ts">
/*
 * FlagDefinitionsModal — visual editor for a template's
 * `flag_definitions`. Mirrors the validation rules the backend enforces
 * in internal/modules/template/flag_definitions.go (max 16, unique
 * uppercase labels, color from the 16-token palette) so the user gets
 * inline feedback before save.
 *
 * Apply is one-way: the parent overwrites draft.flag_definitions on
 * @apply. Cancel discards the working copy.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import Modal from "./Modal.vue";
import SwatchPicker, { type SwatchOption } from "./SwatchPicker.vue";
import { FlagDefinition } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import {
  FLAG_COLORS,
  FLAG_LABEL_REGEX,
  MAX_FLAG_DEFINITIONS,
} from "../utils/flagColors";

// Flag color palette as SwatchPicker options: each FLAG_COLORS entry
// maps to its `.flag-swatch-<name>` CSS class.
const FLAG_SWATCH_OPTIONS: SwatchOption[] = FLAG_COLORS.map((c) => ({
  value: c,
  label: c,
  class: `flag-swatch-${c}`,
}));

const props = defineProps<{
  open: boolean;
  initial: FlagDefinition[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", defs: FlagDefinition[]): void;
}>();

const { t } = useI18n();

// Local working copy. We rebuild from `initial` on each open so a
// previous Cancel can't leak edits back in.
type Draft = { id: number; label: string; color: string };
const draft = ref<Draft[]>([]);
let nextId = 1;

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    draft.value = (props.initial ?? []).map((d) => ({
      id: nextId++,
      label: d.label,
      color: d.color || FLAG_COLORS[0],
    }));
  },
  { immediate: true },
);

// Per-row error code + a global "too many" sentinel. Codes are
// translated by the template; `null` = row is valid.
type RowError = "invalid-label" | "duplicate-label" | "unknown-color" | null;

const errors = computed<RowError[]>(() => {
  const out: RowError[] = [];
  const seen = new Map<string, number>();
  for (const r of draft.value) {
    if (!FLAG_LABEL_REGEX.test(r.label)) {
      out.push("invalid-label");
      continue;
    }
    const prev = seen.get(r.label);
    if (prev !== undefined) {
      out.push("duplicate-label");
      continue;
    }
    seen.set(r.label, out.length);
    if (!FLAG_COLORS.includes(r.color as (typeof FLAG_COLORS)[number])) {
      out.push("unknown-color");
      continue;
    }
    out.push(null);
  }
  return out;
});

const hasErrors = computed(() => errors.value.some((e) => e !== null));
const canSave = computed(() => !hasErrors.value);

function addRow() {
  if (draft.value.length >= MAX_FLAG_DEFINITIONS) return;
  draft.value = [...draft.value, { id: nextId++, label: "", color: FLAG_COLORS[0] }];
}

function removeRow(id: number) {
  draft.value = draft.value.filter((r) => r.id !== id);
}

function setColor(id: number, color: string) {
  draft.value = draft.value.map((r) => (r.id === id ? { ...r, color } : r));
}

function onLabelInput(id: number, value: string) {
  // Auto-uppercase as the user types so they don't trip the regex on
  // the first keystroke.
  const upper = value.toUpperCase();
  draft.value = draft.value.map((r) => (r.id === id ? { ...r, label: upper } : r));
}

function onSave() {
  if (!canSave.value) return;
  const out = draft.value.map(
    (r) => new FlagDefinition({ label: r.label, color: r.color }),
  );
  emit("apply", out);
  emit("close");
}

function onCancel() {
  emit("close");
}

const errorMessage = (code: RowError): string => {
  switch (code) {
    case "duplicate-label":
      return t("flag.builder.error.duplicate_label");
    case "unknown-color":
      return t("flag.builder.error.unknown_color");
    case "invalid-label":
    default:
      // invalid-label has no inline text — the placeholder teaches
      // the format and the row's red border signals the failure.
      return "";
  }
};
</script>

<template>
  <Modal
    :open="open"
    :title="t('flag.builder.title')"
    width="640px"
    @close="onCancel"
  >
    <p class="muted small flag-builder-intro">
      {{ t('flag.builder.intro', [MAX_FLAG_DEFINITIONS]) }}
    </p>

    <div class="flag-builder-counter">
      {{ t('flag.builder.counter', [draft.length, MAX_FLAG_DEFINITIONS]) }}
    </div>

    <p
      v-if="draft.length === 0"
      class="muted small flag-builder-empty"
    >
      {{ t('flag.builder.empty') }}
    </p>

    <draggable
      v-else
      v-model="draft"
      tag="ul"
      class="flag-builder-list"
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      item-key="id"
    >
      <template #item="{ element: row, index: i }">
        <li class="flag-builder-row" :class="{ 'has-error': errors[i] !== null }">
          <span class="dnd-handle" aria-hidden="true">☰</span>

          <SwatchPicker
            :model-value="row.color"
            :options="FLAG_SWATCH_OPTIONS"
            placement="right"
            :cols="4"
            size="22px"
            trigger-class="flag-builder-swatch-trigger"
            :trigger-title="row.color"
            @update:model-value="(v: string) => setColor(row.id, v)"
          />

          <input
            type="text"
            class="field-input flag-builder-label-input"
            :value="row.label"
            :placeholder="t('flag.builder.label_placeholder')"
            :maxlength="32"
            @input="onLabelInput(row.id, ($event.target as HTMLInputElement).value)"
          />

          <span
            v-if="errorMessage(errors[i])"
            class="form-error small flag-builder-row-error"
          >
            {{ errorMessage(errors[i]) }}
          </span>

          <button
            type="button"
            class="tool-btn danger flag-builder-remove"
            :title="t('common.remove')"
            :aria-label="t('common.remove')"
            @click="removeRow(row.id)"
          >×</button>
        </li>
      </template>
    </draggable>

    <div class="flag-builder-add-row">
      <button
        type="button"
        class="tool-btn"
        :disabled="draft.length >= MAX_FLAG_DEFINITIONS"
        @click="addRow"
      >+ {{ t('flag.builder.add') }}</button>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="onCancel">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canSave"
        @click="onSave"
      >{{ t('common.save') }}</button>
    </template>
  </Modal>
</template>
