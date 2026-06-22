<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import FormSection from "./fields/FormSection.vue";
import FieldUnitList from "./FieldUnitList.vue";
import FieldEditModal from "./FieldEditModal.vue";
import ConfirmDialog from "./ConfirmDialog.vue";
import {
  Service as TemplateSvc,
  FieldUnit,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Field, Facet, Formula, SummaryFieldOption } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { recomputeLevelScopes } from "../utils/fieldScopes";

// TemplateFieldsSection
// Owns the "Field Information" panel of the templates workspace:
// the field-tree state, the edit/delete modals, and the section
// markup itself. The parent passes the flat fields slice and listens
// for `update`. The "+ New Field" button (which lives in the
// workspace topbar) calls into the component via defineExpose.
//
// Why a component and not a composable here: the tree/edit/delete
// state is tightly coupled to two modals plus a FormSection. Splitting
// would leave the modal markup orphaned in the parent template -
// chasing handler names through two files instead of one.

const props = defineProps<{
  fields: Field[];
  /** Facets declared on the surrounding template draft. Threaded down
   *  to FieldEditModal so the virtual `facet` field type can bind to
   *  one by key. Defaults to [] when the parent doesn't pass it. */
  facets?: Facet[];
  /** Formulas declared on the surrounding template draft. Threaded down
   *  to FieldEditModal so the virtual `formula` field type can bind its
   *  source. Defaults to [] when the parent doesn't pass it. */
  formulas?: Formula[];
  /** The host template's filename. Threaded down to FieldEditModal so the
   *  api (relation reference) editor scopes its target dropdown to the
   *  template's declared relations. */
  template?: string;
}>();

const emit = defineEmits<{
  update: [flat: Field[]];
  // Fired when an existing field/loop key is renamed, so the parent can offer
  // to migrate stored data from the old key to the new one after saving.
  rename: [oldKey: string, newKey: string];
}>();

const { t } = useI18n();

// ── Field tree (loop blocks as drag units) ───────────────────────────
// Backend owns the tree shape - internal/modules/template/fieldtree.go
// pairs loopstart/loopstop and folds them into one indivisible
// FieldUnit so the editor can't reorder a row across a loop boundary
// by mistake. The flat `props.fields` stays the source of truth; this
// `tree` ref is a view over it that gets rebuilt whenever the parent
// hands in a new array.
const tree = ref<FieldUnit[]>([]);
let lastEmitted: Field[] | null = null;

async function rebuildTree(): Promise<void> {
  tree.value = await TemplateSvc.BuildFieldTree(props.fields ?? []);
}

async function commitTree(): Promise<void> {
  const flat = await TemplateSvc.FlattenFieldTree(tree.value);
  recomputeLevelScopes(flat);
  lastEmitted = flat;
  emit("update", flat);
}

watch(
  () => props.fields,
  (fields) => {
    if (fields && fields === lastEmitted) return;
    void rebuildTree();
  },
  { immediate: true },
);

// ── Field edit / add ─────────────────────────────────────────────────
// editUnit holds the tree-resident FieldUnit being edited (null for
// create). Identity is the JS object reference itself - no key/type
// lookup, no flat-index bookkeeping. applyEdit mutates the unit in
// place and commitTree flushes the resulting tree to the parent.
const editOpen = ref(false);
const editUnit = ref<FieldUnit | null>(null);
const editField = ref<Field | null>(null);
const editIsNew = ref(false);
// Loop summary-field candidates for the field being edited. Backend
// (template.SummaryFieldCandidates) owns loop membership; we fetch the
// direct child fields by the loopstart key whenever a loop is opened.
const summaryFieldOptions = ref<SummaryFieldOption[]>([]);

async function openEdit(u: FieldUnit) {
  if (!u) return;
  editUnit.value = u;
  editField.value = u.kind === "loop" ? (u.start ?? null) : (u.field ?? null);
  summaryFieldOptions.value =
    u.kind === "loop" && u.start?.key
      ? (await TemplateSvc.SummaryFieldCandidates(props.fields ?? [], u.start.key)) ?? []
      : [];
  editIsNew.value = false;
  editOpen.value = true;
}

function openAddField() {
  editUnit.value = null;
  editField.value = null;
  summaryFieldOptions.value = [];
  editIsNew.value = true;
  editOpen.value = true;
}

function applyEdit(updated: Field, originalKey: string) {
  // An existing field/loop whose key changed: tell the parent so it can offer
  // a data migration once the template is saved. New fields never carry data.
  if (!editIsNew.value) {
    const newKey = (updated.key || "").trim();
    const oldKey = (originalKey || "").trim();
    if (oldKey && newKey && oldKey !== newKey) {
      emit("rename", oldKey, newKey);
    }
  }
  if (editIsNew.value) {
    // Looper synth: picking "looper" materialises as a loop unit
    // with a paired loopstart/loopstop sharing the same key/label.
    if (updated.type === "looper") {
      const key = (updated.key || "").trim();
      const label = updated.label || key;
      tree.value.push(new FieldUnit({
        kind: "loop",
        start: { key, label, type: "loopstart" } as Field,
        stop: { key, label, type: "loopstop" } as Field,
        items: [],
      }));
    } else {
      tree.value.push(new FieldUnit({ kind: "field", field: updated }));
    }
    void commitTree();
  } else if (editUnit.value) {
    const u = editUnit.value;
    if (u.kind === "field") {
      u.field = updated;
    } else if (u.kind === "loop" && u.start && u.stop) {
      // loopstart/loopstop share key + label. Keep their types as
      // markers (the modal might not surface those) and propagate
      // any other edits the user made to start.
      const key = (updated.key || "").trim();
      const label = updated.label || key;
      u.start = { ...u.start, ...updated, key, label, type: "loopstart" } as Field;
      u.stop = { ...u.stop, key, label, type: "loopstop" } as Field;
    }
    void commitTree();
  }

  editOpen.value = false;
  editField.value = null;
  editUnit.value = null;
  editIsNew.value = false;
}

const deleteOpen = ref(false);
const deleteUnit = ref<FieldUnit | null>(null);

function openDelete(u: FieldUnit) {
  if (!u) return;
  deleteUnit.value = u;
  deleteOpen.value = true;
}

// Remove the unit identified by reference from anywhere in the tree.
// Returns true on success. Caller is responsible for committing.
function removeUnitByRef(units: FieldUnit[], target: FieldUnit): boolean {
  const idx = units.indexOf(target);
  if (idx !== -1) {
    units.splice(idx, 1);
    return true;
  }
  for (const u of units) {
    if (u.kind === "loop" && u.items) {
      if (removeUnitByRef(u.items, target)) return true;
    }
  }
  return false;
}

const deleteFieldName = ref("");
watch(deleteUnit, (u) => {
  if (!u) {
    deleteFieldName.value = "";
    return;
  }
  if (u.kind === "loop") {
    deleteFieldName.value = u.start?.label || u.start?.key || "";
  } else {
    deleteFieldName.value = u.field?.label || u.field?.key || "";
  }
});

function confirmDelete() {
  const u = deleteUnit.value;
  if (!u) {
    deleteOpen.value = false;
    deleteUnit.value = null;
    return;
  }
  // Deleting a loop unit removes the whole unit (start + items + stop)
  // in one step - no separate loopstart/loopstop pair-removal walk is
  // needed. Orphan markers (rendered as plain field rows) drop
  // individually, which is what the user wants.
  removeUnitByRef(tree.value, u);
  void commitTree();
  deleteOpen.value = false;
  deleteUnit.value = null;
}

defineExpose({ openAddField });
</script>

<template>
  <FormSection :title="t('workspace.templates.fields.title')">
    <div class="fields-content">
      <p v-if="!props.fields || props.fields.length === 0" class="muted small">
        {{ t('workspace.templates.fields.empty') }}
      </p>
      <FieldUnitList
        v-else
        :units="tree"
        :depth="0"
        @change="commitTree"
        @edit-unit="openEdit"
        @delete-unit="openDelete"
      />
    </div>
  </FormSection>

  <FieldEditModal
    :open="editOpen"
    :field="editField"
    :is-new="editIsNew"
    :available-facets="facets ?? []"
    :available-formulas="formulas ?? []"
    :available-fields="fields ?? []"
    :host-template="template ?? ''"
    :summary-field-options="summaryFieldOptions"
    @close="editOpen = false"
    @confirm="applyEdit"
  />

  <ConfirmDialog
    :open="deleteOpen"
    :title="t('workspace.templates.field_edit.delete_title')"
    :message="t('workspace.templates.field_edit.delete_confirm', [deleteFieldName])"
    :confirm-label="t('workspace.profiles.action.delete')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="deleteOpen = false"
    @confirm="confirmDelete"
  />
</template>
