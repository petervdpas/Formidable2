<script setup lang="ts">
/*
 * RelationEditorModal - visual editor for ONE template-to-template relation
 * (target template + cardinality). The parent owns the relations list and the
 * option sources, and routes one edit at a time here; Apply emits the Relation.
 *
 * Backend steers: both the target options and the cardinality options (value +
 * label) are supplied by the parent from backend sources. This modal renders
 * them and keeps no option set or label mapping of its own.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { SelectField, SwitchField } from "./fields";
import {
  Relation,
  Cardinality,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation";

const props = defineProps<{
  open: boolean;
  initial: Relation;
  isEdit?: boolean;
  /** Selectable target templates as {value: filename, label: name}. */
  targets: { value: string; label: string }[];
  /** Cardinality options as {value, label}, localized by the parent. */
  cardinalities: { value: string; label: string }[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", relation: Relation): void;
}>();

const { t } = useI18n();

const draftTarget = ref("");
const draftCardinality = ref("");
const draftInverse = ref(false);

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    draftTarget.value = props.initial.to ?? "";
    draftCardinality.value = props.initial.cardinality ?? "";
    draftInverse.value = props.initial.inverse ?? false;
  },
  { immediate: true },
);

const canSave = computed(
  () => draftTarget.value !== "" && draftCardinality.value !== "",
);

function onSave() {
  if (!canSave.value) return;
  emit(
    "apply",
    new Relation({
      to: draftTarget.value,
      cardinality: draftCardinality.value as Cardinality,
      inverse: draftInverse.value,
    }),
  );
  emit("close");
}
function onCancel() {
  emit("close");
}
</script>

<template>
  <Modal
    :open="open"
    :title="isEdit
      ? t('workspace.templates.relations.editor.title_edit')
      : t('workspace.templates.relations.editor.title_add')"
    width="520px"
    @close="onCancel"
  >
    <div class="relation-editor">
      <div class="relation-editor-field">
        <span class="relation-editor-label">
          {{ t('workspace.templates.relations.editor.target_label') }}
        </span>
        <SelectField
          v-model="draftTarget"
          :options="targets"
          :placeholder="t('workspace.templates.relations.editor.target_placeholder')"
        />
      </div>
      <div class="relation-editor-field">
        <span class="relation-editor-label">
          {{ t('workspace.templates.relations.editor.cardinality_label') }}
        </span>
        <SelectField
          v-model="draftCardinality"
          :options="cardinalities"
        />
      </div>
      <div class="relation-editor-field">
        <span class="relation-editor-label">
          {{ t('workspace.templates.relations.editor.inverse_label') }}
        </span>
        <SwitchField v-model="draftInverse" />
      </div>
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
