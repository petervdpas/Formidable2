<script setup lang="ts">
/*
 * TemplateFormulasTab - the Formulas setup tab. Presentational: the parent owns
 * the template draft; this renders the formulas list and emits update:formulas.
 * Owns its editor modal + delete confirm.
 */
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import FormulaEditorModal from "./FormulaEditorModal.vue";
import ConfirmDialog from "./ConfirmDialog.vue";
import {
  Formula,
  type Field,
  type Facet,
  type Scaling,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  template: string;
  formulas: Formula[];
  fields: Field[];
  facets: Facet[];
  scalings: Scaling[];
}>();
const emit = defineEmits<{ (e: "update:formulas", v: Formula[]): void }>();

const { t } = useI18n();

const formulasModel = computed<Formula[]>({
  get: () => props.formulas ?? [],
  set: (v) => emit("update:formulas", v),
});

const editorOpen = ref(false);
const editingIndex = ref(-1);
const editing = ref<Formula | null>(null);

function openAdd() {
  editingIndex.value = -1;
  editing.value = null;
  editorOpen.value = true;
}
function openEdit(idx: number) {
  const f = props.formulas?.[idx];
  if (!f) return;
  editingIndex.value = idx;
  editing.value = new Formula({ key: f.key, label: f.label, type: f.type, expression: f.expression });
  editorOpen.value = true;
}
function apply(f: Formula) {
  const cur = props.formulas ?? [];
  emit(
    "update:formulas",
    editingIndex.value < 0
      ? [...cur, f]
      : cur.map((existing, i) => (i === editingIndex.value ? f : existing)),
  );
  editorOpen.value = false;
}

const confirmOpen = ref(false);
const pendingIndex = ref(-1);
const pendingLabel = computed(() => {
  const f = props.formulas?.[pendingIndex.value];
  return f ? f.label || f.key : "";
});
function askRemove(idx: number) {
  pendingIndex.value = idx;
  confirmOpen.value = true;
}
function confirmRemove() {
  const idx = pendingIndex.value;
  confirmOpen.value = false;
  pendingIndex.value = -1;
  if (idx >= 0) {
    const cur = props.formulas ?? [];
    emit("update:formulas", [...cur.slice(0, idx), ...cur.slice(idx + 1)]);
  }
}
function cancelRemove() {
  confirmOpen.value = false;
  pendingIndex.value = -1;
}
</script>

<template>
  <div class="setup-tab-pane">
    <p v-if="formulasModel.length === 0" class="muted small">
      {{ t('workspace.templates.formulas.empty') }}
    </p>
    <draggable
      v-else
      v-model="formulasModel"
      tag="ul"
      class="formula-rows"
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      item-key="key"
    >
      <template #item="{ element: f, index: i }">
        <li class="formula-row list-card">
          <span class="dnd-handle" aria-hidden="true">☰</span>
          <span class="formula-row-name">{{ f.label || f.key }}</span>
          <span class="formula-row-type">{{ f.type }}</span>
          <code class="formula-row-expr">{{ f.expression }}</code>
          <button
            class="tool-btn"
            type="button"
            :title="t('workspace.templates.formulas.edit')"
            @click="openEdit(i)"
          >{{ t('workspace.templates.formulas.edit') }}</button>
          <button
            class="tool-btn danger"
            type="button"
            :title="t('workspace.templates.formulas.remove')"
            @click="askRemove(i)"
          >×</button>
        </li>
      </template>
    </draggable>
    <div class="setup-tab-actions">
      <button class="tool-btn" type="button" @click="openAdd">
        + {{ t('workspace.templates.formulas.add') }}
      </button>
    </div>

    <FormulaEditorModal
      :open="editorOpen"
      :template="template"
      :fields="fields"
      :facets="facets"
      :scalings="scalings"
      :initial="editing"
      @close="editorOpen = false"
      @apply="apply"
    />
    <ConfirmDialog
      :open="confirmOpen"
      :title="t('workspace.templates.formulas.remove_title')"
      :message="t('workspace.templates.formulas.remove_confirm', [pendingLabel])"
      :confirm-label="t('workspace.templates.formulas.remove')"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @confirm="confirmRemove"
      @cancel="cancelRemove"
    />
  </div>
</template>
