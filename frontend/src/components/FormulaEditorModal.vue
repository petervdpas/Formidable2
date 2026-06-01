<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { TextField, TextareaField, SelectField } from "./fields";
import { Formula, Facet, Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { FormulaService } from "../../bindings/github.com/petervdpas/formidable2/internal/app";
import { backendErrMessage } from "../utils/backendError";

const props = defineProps<{
  open: boolean;
  template: string;
  fields: Field[];
  facets: Facet[];
  initial: Formula | null;
}>();
const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", formula: Formula): void;
}>();

const { t } = useI18n();

const key = ref("");
const label = ref("");
const type = ref("number");
const expression = ref("");
const preview = ref("");
const previewError = ref("");

const typeOptions = computed(() => [
  { value: "number", label: t("workspace.templates.formulas.type.number") },
  { value: "text", label: t("workspace.templates.formulas.type.text") },
  { value: "date", label: t("workspace.templates.formulas.type.date") },
  { value: "bool", label: t("workspace.templates.formulas.type.bool") },
]);

// Field/facet reference chips: clicking one appends its F["key"] token. Skip
// presentation-only field types that carry no value (mirrors the loader skip).
const skipTypes = new Set(["image", "api", "button", "facet", "heading"]);
const refTokens = computed(() => {
  const out: { token: string; label: string }[] = [];
  for (const f of props.fields ?? []) {
    if (skipTypes.has(f.type)) continue;
    out.push({ token: `F["${f.key}"]`, label: f.label || f.key });
  }
  for (const fc of props.facets ?? []) {
    out.push({ token: `F["${fc.key}"]`, label: fc.key });
  }
  return out;
});

function insertToken(token: string) {
  expression.value = expression.value ? `${expression.value} ${token}` : token;
}

const canApply = computed(() => key.value.trim() !== "" && expression.value.trim() !== "");

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    key.value = props.initial?.key ?? "";
    label.value = props.initial?.label ?? "";
    type.value = props.initial?.type || "number";
    expression.value = props.initial?.expression ?? "";
    preview.value = "";
    previewError.value = "";
  },
);

// Live preview against the template's first stored form. Debounced so each
// keystroke doesn't round-trip; the backend builds the same context the chart
// will, so this is the real value, not a client-side guess.
let previewTimer: ReturnType<typeof setTimeout> | undefined;
watch([expression, type], () => {
  if (previewTimer) clearTimeout(previewTimer);
  previewError.value = "";
  if (!props.template || expression.value.trim() === "") {
    preview.value = "";
    return;
  }
  previewTimer = setTimeout(async () => {
    try {
      preview.value = await FormulaService.Preview(props.template, expression.value, type.value);
    } catch (e) {
      preview.value = "";
      previewError.value = backendErrMessage(e);
    }
  }, 300);
});

function apply() {
  if (!canApply.value) return;
  emit("apply", new Formula({
    key: key.value.trim(),
    label: label.value.trim(),
    type: type.value,
    expression: expression.value.trim(),
  }));
}
</script>

<template>
  <Modal :open="open" :title="t('workspace.templates.formulas.title')" width="640px" @close="emit('close')">
    <div class="formula-editor">
      <div class="formula-editor-row">
        <label class="formula-editor-field">
          <span class="formula-editor-label">{{ t('workspace.templates.formulas.key') }}</span>
          <TextField v-model="key" :placeholder="t('workspace.templates.formulas.key_placeholder')" />
        </label>
        <label class="formula-editor-field">
          <span class="formula-editor-label">{{ t('workspace.templates.formulas.label') }}</span>
          <TextField v-model="label" />
        </label>
        <label class="formula-editor-field formula-editor-field-type">
          <span class="formula-editor-label">{{ t('workspace.templates.formulas.type') }}</span>
          <SelectField v-model="type" :options="typeOptions" />
        </label>
      </div>

      <span class="formula-editor-label">{{ t('workspace.templates.formulas.expression') }}</span>
      <p class="muted small formula-editor-hint">{{ t('workspace.templates.formulas.expression_hint') }}</p>
      <TextareaField v-model="expression" :rows="4" placeholder='F["amount"] * 0.21' />

      <div class="formula-editor-refs">
        <button
          v-for="r in refTokens"
          :key="r.token"
          type="button"
          class="formula-ref-chip"
          :title="r.token"
          @click="insertToken(r.token)"
        >{{ r.label }}</button>
      </div>

      <div class="formula-editor-preview">
        <span class="formula-editor-label">{{ t('workspace.templates.formulas.preview') }}</span>
        <code v-if="previewError" class="formula-preview-error">{{ previewError }}</code>
        <code v-else-if="preview !== ''" class="formula-preview-value">{{ preview }}</code>
        <span v-else class="muted small">{{ t('workspace.templates.formulas.preview_empty') }}</span>
      </div>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button class="tool-btn primary" type="button" :disabled="!canApply" @click="apply">
        {{ t('common.apply') }}
      </button>
    </template>
  </Modal>
</template>
