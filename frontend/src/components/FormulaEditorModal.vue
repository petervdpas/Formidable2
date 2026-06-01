<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { TextField, SelectField } from "./fields";
import { Formula, Facet, Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { FormulaService } from "../../bindings/github.com/petervdpas/formidable2/internal/app";
import { Service as ExpressionSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
import type { FunctionDoc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/models";
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
const exprArea = ref<HTMLTextAreaElement | null>(null);

const typeOptions = computed(() => [
  { value: "number", label: t("workspace.templates.formulas.type.number") },
  { value: "text", label: t("workspace.templates.formulas.type.text") },
  { value: "date", label: t("workspace.templates.formulas.type.date") },
  { value: "bool", label: t("workspace.templates.formulas.type.bool") },
]);

// Field/facet references, deduped, skipping presentation/structural types.
const skipTypes = new Set([
  "image", "api", "button", "facet", "heading", "loopstart", "loopstop",
]);
const refTokens = computed(() => {
  const out: { token: string; label: string }[] = [];
  const seen = new Set<string>();
  const add = (k: string, l: string) => {
    if (!k || seen.has(k)) return;
    seen.add(k);
    out.push({ token: `F["${k}"]`, label: l });
  };
  for (const f of props.fields ?? []) {
    if (skipTypes.has(f.type)) continue;
    add(f.key, f.label || f.key);
  }
  for (const fc of props.facets ?? []) add(fc.key, fc.key);
  return out;
});

// Function/control catalog from the backend, so the palettes reflect the
// engine's real capabilities rather than a hardcoded list.
const functions = ref<FunctionDoc[]>([]);
const fnCategoryKeys: Record<string, string> = {
  math: "workspace.templates.formulas.fn_math",
  date: "workspace.templates.formulas.fn_date",
  text: "workspace.templates.formulas.fn_text",
};
const fnGroups = computed(() =>
  ["math", "date", "text"].map((cat) => ({
    label: t(fnCategoryKeys[cat]),
    items: functions.value.filter((f) => f.category === cat),
  })).filter((g) => g.items.length > 0),
);
const controlFns = computed(() => functions.value.filter((f) => f.category === "control"));

// Insert text at the textarea cursor (replacing any selection), then re-focus
// and place the caret after it, so building an expression doesn't fight you.
function insertSnippet(text: string) {
  const el = exprArea.value;
  if (!el) {
    expression.value = expression.value ? `${expression.value} ${text}` : text;
    return;
  }
  const start = el.selectionStart ?? expression.value.length;
  const end = el.selectionEnd ?? start;
  expression.value = expression.value.slice(0, start) + text + expression.value.slice(end);
  void nextTick(() => {
    el.focus();
    const pos = start + text.length;
    el.setSelectionRange(pos, pos);
  });
}

const canApply = computed(() => key.value.trim() !== "" && expression.value.trim() !== "");

watch(
  () => props.open,
  async (open) => {
    if (!open) return;
    key.value = props.initial?.key ?? "";
    label.value = props.initial?.label ?? "";
    type.value = props.initial?.type || "number";
    expression.value = props.initial?.expression ?? "";
    preview.value = "";
    previewError.value = "";
    evaluated.value = false;
    if (functions.value.length === 0) {
      try {
        functions.value = await ExpressionSvc.Functions();
      } catch {
        functions.value = [];
      }
    }
  },
);

// Preview is on demand (an Evaluate button), not live, so the dialog doesn't
// reflow on every keystroke. The backend builds the same context the chart
// will, so the value shown is the real one.
const evaluating = ref(false);
const evaluated = ref(false); // true once a run completed, so "" reads as an empty result, not "not run"
const canEvaluate = computed(() => props.template !== "" && expression.value.trim() !== "");

// Editing invalidates the shown result; revert to the hint until re-evaluated.
watch(expression, () => {
  evaluated.value = false;
  previewError.value = "";
});

async function evaluate() {
  if (!canEvaluate.value) return;
  evaluating.value = true;
  previewError.value = "";
  try {
    preview.value = await FormulaService.Preview(props.template, expression.value, type.value);
    evaluated.value = true;
  } catch (e) {
    preview.value = "";
    evaluated.value = false;
    previewError.value = backendErrMessage(e);
  } finally {
    evaluating.value = false;
  }
}

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
      <textarea
        ref="exprArea"
        v-model="expression"
        class="formula-expr-input"
        rows="4"
        spellcheck="false"
        placeholder='F["amount"] * 0.21'
      ></textarea>

      <!-- Insert palettes: field references, functions (grouped), control. -->
      <div class="formula-palette">
        <span class="formula-editor-label">{{ t('workspace.templates.formulas.insert') }}</span>
        <select
          class="formula-palette-select"
          :value="''"
          @change="insertSnippet(($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''"
        >
          <option value="">{{ t('workspace.templates.formulas.palette_field') }}</option>
          <option v-for="r in refTokens" :key="r.token" :value="r.token">{{ r.label }}</option>
        </select>
        <select
          class="formula-palette-select"
          :value="''"
          @change="insertSnippet(($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''"
        >
          <option value="">{{ t('workspace.templates.formulas.palette_function') }}</option>
          <optgroup v-for="g in fnGroups" :key="g.label" :label="g.label">
            <option v-for="fn in g.items" :key="fn.name" :value="fn.snippet" :title="fn.description">
              {{ fn.name }}
            </option>
          </optgroup>
        </select>
        <select
          class="formula-palette-select"
          :value="''"
          @change="insertSnippet(($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''"
        >
          <option value="">{{ t('workspace.templates.formulas.palette_control') }}</option>
          <option v-for="fn in controlFns" :key="fn.name" :value="fn.snippet" :title="fn.description">
            {{ fn.name }}
          </option>
        </select>
      </div>

      <div class="formula-editor-preview-head">
        <span class="formula-editor-label">{{ t('workspace.templates.formulas.preview') }}</span>
        <button
          class="tool-btn"
          type="button"
          :disabled="!canEvaluate || evaluating"
          @click="evaluate"
        >{{ t('workspace.templates.formulas.evaluate') }}</button>
      </div>
      <div class="formula-preview-box">
        <code v-if="previewError" class="formula-preview-error">{{ previewError }}</code>
        <code v-else-if="evaluated && preview !== ''" class="formula-preview-value">{{ preview }}</code>
        <span v-else-if="evaluated" class="muted small">{{ t('workspace.templates.formulas.preview_empty_result') }}</span>
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
