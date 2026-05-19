<script setup lang="ts">
/*
 * FacetEditorModal — visual editor for ONE facet (key + icon + options).
 * Backend (internal/modules/template/facets.go) owns every constraint;
 * we fetch them via useFacetMeta and render whatever the backend sends.
 *
 * The parent owns the facets list and routes one edit at a time here.
 * Apply emits the edited Facet; Cancel discards the working copy.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import Modal from "./Modal.vue";
import SwatchPicker, { type SwatchOption } from "./SwatchPicker.vue";
import {
  Facet,
  FacetOption,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { useFacetMeta } from "../composables/useFacetMeta";

const {
  colors: backendColors,
  icons: backendIcons,
  keyRegex,
  labelRegex,
  maxOptionsPerFacet,
} = useFacetMeta();

const colorSwatchOptions = computed<SwatchOption[]>(() =>
  backendColors.value.map((c) => ({
    value: c,
    label: c,
    class: `facet-swatch-${c}`,
  })),
);

const iconSwatchOptions = computed<SwatchOption[]>(() =>
  backendIcons.value.map((i) => ({
    value: i,
    label: i.replace(/^fa-/, ""),
    icon: `fa-solid ${i}`,
  })),
);

const defaultColor = computed(() => backendColors.value[0] ?? "red");
const defaultIcon = computed(() => backendIcons.value[0] ?? "fa-flag");

const props = defineProps<{
  open: boolean;
  initial: Facet;
  /** Keys of other facets on this template — used for uniqueness check.
   *  Exclude the key being edited so renaming-to-self is OK. */
  existingKeys?: string[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", facet: Facet): void;
}>();

const { t } = useI18n();

type DraftOption = { id: number; label: string; color: string };
const draftKey = ref("");
const draftIcon = ref<string>("");
const draftOptions = ref<DraftOption[]>([]);
let nextId = 1;

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    draftKey.value = props.initial.key ?? "";
    draftIcon.value = props.initial.icon || defaultIcon.value;
    draftOptions.value = (props.initial.options ?? []).map((o) => ({
      id: nextId++,
      label: o.label,
      color: o.color || defaultColor.value,
    }));
  },
  { immediate: true },
);

const keyError = computed<"empty" | "invalid" | "duplicate" | null>(() => {
  if (draftKey.value === "") return "empty";
  if (!keyRegex.value.test(draftKey.value)) return "invalid";
  if ((props.existingKeys ?? []).includes(draftKey.value)) return "duplicate";
  return null;
});

const iconError = computed<"empty" | "unknown" | null>(() => {
  if (!draftIcon.value) return "empty";
  if (!backendIcons.value.includes(draftIcon.value)) return "unknown";
  return null;
});

type RowError = "invalid-label" | "duplicate-label" | "unknown-color" | null;

const optionErrors = computed<RowError[]>(() => {
  const out: RowError[] = [];
  const seen = new Set<string>();
  for (const r of draftOptions.value) {
    if (!labelRegex.value.test(r.label)) {
      out.push("invalid-label");
      continue;
    }
    if (seen.has(r.label)) {
      out.push("duplicate-label");
      continue;
    }
    seen.add(r.label);
    if (!backendColors.value.includes(r.color)) {
      out.push("unknown-color");
      continue;
    }
    out.push(null);
  }
  return out;
});

const hasOptionErrors = computed(() => optionErrors.value.some((e) => e !== null));
const noOptions = computed(() => draftOptions.value.length === 0);
const canSave = computed(
  () =>
    keyError.value === null &&
    iconError.value === null &&
    !noOptions.value &&
    !hasOptionErrors.value,
);

function addRow() {
  if (draftOptions.value.length >= maxOptionsPerFacet.value) return;
  draftOptions.value = [
    ...draftOptions.value,
    { id: nextId++, label: "", color: defaultColor.value },
  ];
}

function removeRow(id: number) {
  draftOptions.value = draftOptions.value.filter((r) => r.id !== id);
}

function setColor(id: number, color: string) {
  draftOptions.value = draftOptions.value.map((r) =>
    r.id === id ? { ...r, color } : r,
  );
}

function onLabelInput(id: number, value: string) {
  const upper = value.toUpperCase();
  draftOptions.value = draftOptions.value.map((r) =>
    r.id === id ? { ...r, label: upper } : r,
  );
}

function onKeyInput(value: string) {
  draftKey.value = value.toLowerCase();
}

function onSave() {
  if (!canSave.value) return;
  const out = new Facet({
    key: draftKey.value,
    icon: draftIcon.value,
    options: draftOptions.value.map(
      (r) => new FacetOption({ label: r.label, color: r.color }),
    ),
  });
  emit("apply", out);
  emit("close");
}

function onCancel() {
  emit("close");
}

const optionErrorMessage = (code: RowError): string => {
  switch (code) {
    case "duplicate-label":
      return t("facet.builder.error.duplicate_label");
    case "unknown-color":
      return t("facet.builder.error.unknown_color");
    case "invalid-label":
    default:
      return "";
  }
};

const keyErrorMessage = computed(() => {
  switch (keyError.value) {
    case "empty":
      return t("facet.builder.error.key_empty");
    case "invalid":
      return t("facet.builder.error.key_invalid");
    case "duplicate":
      return t("facet.builder.error.key_duplicate");
    default:
      return "";
  }
});
</script>

<template>
  <Modal
    :open="open"
    :title="t('facet.builder.title')"
    width="640px"
    @close="onCancel"
  >
    <p class="muted small facet-builder-intro">
      {{ t('facet.builder.intro', [maxOptionsPerFacet]) }}
    </p>

    <div class="facet-builder-header">
      <label class="facet-builder-header-row">
        <span class="facet-builder-header-label">{{ t('facet.builder.icon_label') }}</span>
        <SwatchPicker
          :model-value="draftIcon"
          :options="iconSwatchOptions"
          placement="right"
          :cols="4"
          size="44px"
          trigger-class="facet-builder-icon-trigger"
          :trigger-title="draftIcon"
          teleport
          @update:model-value="(v: string) => (draftIcon = v)"
        />
      </label>

      <label class="facet-builder-header-row">
        <span class="facet-builder-header-label">{{ t('facet.builder.key_label') }}</span>
        <input
          type="text"
          class="field-input facet-builder-key-input"
          :class="{ 'has-error': keyError !== null }"
          :value="draftKey"
          :placeholder="t('facet.builder.key_placeholder')"
          :maxlength="32"
          @input="onKeyInput(($event.target as HTMLInputElement).value)"
        />
        <span
          v-if="keyErrorMessage"
          class="form-error small facet-builder-row-error"
        >
          {{ keyErrorMessage }}
        </span>
      </label>
    </div>

    <div class="facet-builder-counter">
      {{ t('facet.builder.counter', [draftOptions.length, maxOptionsPerFacet]) }}
    </div>

    <p
      v-if="noOptions"
      class="muted small facet-builder-empty"
    >
      {{ t('facet.builder.empty') }}
    </p>

    <draggable
      v-else
      v-model="draftOptions"
      tag="ul"
      class="facet-builder-list"
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      item-key="id"
    >
      <template #item="{ element: row, index: i }">
        <li class="facet-builder-row" :class="{ 'has-error': optionErrors[i] !== null }">
          <span class="dnd-handle" aria-hidden="true">☰</span>

          <SwatchPicker
            :model-value="row.color"
            :options="colorSwatchOptions"
            placement="right"
            :cols="4"
            size="22px"
            trigger-class="facet-builder-swatch-trigger"
            :trigger-title="row.color"
            teleport
            @update:model-value="(v: string) => setColor(row.id, v)"
          />

          <input
            type="text"
            class="field-input facet-builder-label-input"
            :value="row.label"
            :placeholder="t('facet.builder.label_placeholder')"
            :maxlength="32"
            @input="onLabelInput(row.id, ($event.target as HTMLInputElement).value)"
          />

          <span
            v-if="optionErrorMessage(optionErrors[i])"
            class="form-error small facet-builder-row-error"
          >
            {{ optionErrorMessage(optionErrors[i]) }}
          </span>

          <button
            type="button"
            class="tool-btn danger facet-builder-remove"
            :title="t('common.remove')"
            :aria-label="t('common.remove')"
            @click="removeRow(row.id)"
          >×</button>
        </li>
      </template>
    </draggable>

    <div class="facet-builder-add-row">
      <button
        type="button"
        class="tool-btn"
        :disabled="draftOptions.length >= maxOptionsPerFacet"
        @click="addRow"
      >+ {{ t('facet.builder.add_option') }}</button>
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
