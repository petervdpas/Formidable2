<script setup lang="ts">
/*
 * RelationEditorModal - visual editor for ONE template-to-template relation
 * (target template + cardinality). The parent owns the relations list and
 * routes one edit at a time here; Apply emits the edited Relation.
 *
 * Backend steers: the cardinality option set comes from Relation.Cardinalities,
 * and the target options are supplied by the parent from ListCollectionTemplates.
 */
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { SelectField } from "./fields";
import {
  Service as RelationSvc,
  Relation,
  Cardinality,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation";

const props = defineProps<{
  open: boolean;
  initial: Relation;
  isEdit?: boolean;
  /** Selectable target templates as {value: filename, label: name}. */
  targets: { value: string; label: string }[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", relation: Relation): void;
}>();

const { t } = useI18n();

// Explicit key map (no interpolated i18n keys).
const CARDINALITY_LABEL_KEYS = {
  [Cardinality.OneToOne]: "workspace.templates.relations.cardinality.one_to_one",
  [Cardinality.OneToMany]: "workspace.templates.relations.cardinality.one_to_many",
  [Cardinality.ManyToMany]: "workspace.templates.relations.cardinality.many_to_many",
} as const;

const cardinalities = ref<Cardinality[]>([]);
onMounted(async () => {
  cardinalities.value = (await RelationSvc.Cardinalities()) ?? [];
});
const cardinalityOptions = computed(() =>
  cardinalities.value.map((c) => {
    const key = CARDINALITY_LABEL_KEYS[c];
    return { value: c, label: key ? t(key) : c };
  }),
);

const draftTarget = ref("");
const draftCardinality = ref<Cardinality>(Cardinality.OneToMany);

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    draftTarget.value = props.initial.to ?? "";
    draftCardinality.value =
      (props.initial.cardinality as Cardinality) || Cardinality.OneToMany;
  },
  { immediate: true },
);

const canSave = computed(
  () => draftTarget.value !== "" && draftCardinality.value !== Cardinality.$zero,
);

function onSave() {
  if (!canSave.value) return;
  emit(
    "apply",
    new Relation({ to: draftTarget.value, cardinality: draftCardinality.value }),
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
          :model-value="draftCardinality"
          :options="cardinalityOptions"
          @update:model-value="(v: string) => (draftCardinality = v as Cardinality)"
        />
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
