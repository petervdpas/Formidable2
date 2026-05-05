<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import SplitPane from "../components/SplitPane.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import FieldEditModal from "../components/FieldEditModal.vue";
import CodeEditor from "../components/CodeEditor.vue";
import {
  FormSection,
  FormRow,
  TextField,
  TextareaField,
  SelectField,
  SwitchField,
} from "../components/fields";
import { useTemplates, isValidTemplateFilename } from "../composables/useTemplates";
import { useTemplateEditor } from "../composables/useTemplateEditor";
import { useRestartGate } from "../composables/useRestartGate";
import { useToast } from "../composables/useToast";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useConfig } from "../composables/useConfig";
import { watch } from "vue";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { update: updateConfig } = useConfig();
const toast = useToast();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const {
  filenames,
  cache,
  selectedFilename,
  selectedTemplate,
  refresh,
  create,
  remove,
} = useTemplates();

const { draft, dirty, itemFieldOptions, save, reset } = useTemplateEditor();

// Restore the persisted template selection once the filename list is
// populated. Watch the list (not bootConfig) because templates may
// load before or after config — whichever comes second triggers this.
let selectionRestored = false;
watch(
  () => filenames.value,
  (list) => {
    if (selectionRestored) return;
    if (!list.length) return;
    const want = bootConfig.value?.selected_template;
    if (want && list.includes(want)) {
      selectedFilename.value = want;
    }
    selectionRestored = true;
  },
  { immediate: true },
);

// Persist the user's selection — but only after restore has run, so we
// don't overwrite the saved value with an empty initial state.
watch(selectedFilename, (fn) => {
  if (!selectionRestored) return;
  void updateConfig({ selected_template: fn });
});

function displayName(filename: string): string {
  const cached = cache.value.get(filename);
  if (cached?.name && cached.name.trim()) return cached.name;
  return filename.replace(/\.yaml$/, "");
}

// ── Refresh feedback ──────────────────────────────────────────────────
async function doRefresh() {
  try {
    await refresh();
    toast.success("toast.refresh.success");
  } catch (err) {
    toast.error("toast.refresh.error", [String(err)]);
  }
}

// ── Save / Reset ─────────────────────────────────────────────────────
async function doSave() {
  if (!draft.value || !selectedFilename.value) return;
  const result = await save();
  if (result.ok) {
    toast.success(
      "workspace.templates.save_success",
      [draft.value?.name || selectedFilename.value],
    );
  } else {
    toast.error("workspace.templates.save_error", [result.message ?? "?"]);
  }
}

function doReset() {
  reset();
}

// ── Item Field options for the Setup dropdown ─────────────────────────
const itemFieldSelectOptions = computed(() => {
  const opts: { value: string; label: string }[] = [
    { value: "", label: t("workspace.templates.item_field_none") },
  ];
  for (const f of itemFieldOptions.value) {
    opts.push({ value: f.key, label: `${f.label} (${f.key})` });
  }
  return opts;
});

// ── Create modal ─────────────────────────────────────────────────────
const createOpen = ref(false);
const createInput = ref("");
const createError = ref<string>("");

function openCreate() {
  createInput.value = "";
  createError.value = "";
  createOpen.value = true;
}

async function submitCreate() {
  const name = createInput.value.trim();
  if (!isValidTemplateFilename(name)) {
    createError.value = t("workspace.templates.create.invalid");
    return;
  }
  const result = await create(name);
  if (!result.ok) {
    createError.value = result.code === "exists"
      ? t("workspace.templates.create.exists")
      : t("workspace.templates.create.error", [result.message ?? "?"]);
    return;
  }
  toast.success("workspace.templates.create.success", [name.replace(/\.yaml$/, "")]);
  createOpen.value = false;
}

// ── Field edit / add ─────────────────────────────────────────────────
// editIndex === -1 means "creating" (no existing field at that index);
// editField is the field being edited, or null for create.
const editOpen = ref(false);
const editIndex = ref<number>(-1);
const editField = ref<Field | null>(null);
const editIsNew = ref(false);

function openEdit(index: number) {
  if (!draft.value || !draft.value.fields) return;
  editIndex.value = index;
  editField.value = draft.value.fields[index] ?? null;
  editIsNew.value = false;
  editOpen.value = true;
}

function openAddField() {
  if (!draft.value) return;
  editIndex.value = -1;
  editField.value = null;
  editIsNew.value = true;
  editOpen.value = true;
}

function applyEdit(updated: Field) {
  if (!draft.value) return;
  const fields = draft.value.fields ?? [];

  // Looper synthesis — picking "looper" creates a loopstart/loopstop
  // pair sharing the same key/label. Only valid in create mode.
  if (updated.type === "looper") {
    const key = (updated.key || "").trim();
    const label = updated.label || key;
    const start = { key, label, type: "loopstart" } as Field;
    const stop = { key, label, type: "loopstop" } as Field;
    if (editIsNew.value) {
      draft.value.fields = [...fields, start, stop];
    }
  } else if (editIsNew.value) {
    // Append a fresh field at the end.
    draft.value.fields = [...fields, updated];
  } else if (editIndex.value >= 0) {
    // In-place replace.
    draft.value.fields = [
      ...fields.slice(0, editIndex.value),
      updated,
      ...fields.slice(editIndex.value + 1),
    ];
  }

  editOpen.value = false;
  editField.value = null;
  editIndex.value = -1;
  editIsNew.value = false;
}

const deleteOpen = ref(false);
const deleteIndex = ref<number>(-1);

function openDelete(index: number) {
  deleteIndex.value = index;
  deleteOpen.value = true;
}

const deleteFieldName = computed(() => {
  if (!draft.value || deleteIndex.value < 0) return "";
  const f = draft.value.fields[deleteIndex.value];
  return f?.label || f?.key || "";
});

function confirmDelete() {
  if (!draft.value || deleteIndex.value < 0) {
    deleteOpen.value = false;
    return;
  }
  const idx = deleteIndex.value;
  const fields = draft.value.fields;
  const removed = fields[idx];
  let next = [...fields.slice(0, idx), ...fields.slice(idx + 1)];

  // Loopstart/loopstop pairing — if we removed one half, drop its
  // partner so the YAML stays valid.
  if (removed && (removed.type === "loopstart" || removed.type === "loopstop")) {
    const partnerType = removed.type === "loopstart" ? "loopstop" : "loopstart";
    const partnerIdx = next.findIndex(
      (f) => f.key === removed.key && f.type === partnerType,
    );
    if (partnerIdx !== -1) {
      next = [...next.slice(0, partnerIdx), ...next.slice(partnerIdx + 1)];
    }
  }
  draft.value.fields = next;
  deleteOpen.value = false;
  deleteIndex.value = -1;
}

// ── Delete template ──────────────────────────────────────────────────
const deleteTplOpen = ref(false);

function openDeleteTemplate() {
  if (!selectedFilename.value) return;
  deleteTplOpen.value = true;
}

const deleteTplName = computed(() => {
  const f = selectedFilename.value;
  if (!f) return "";
  return displayName(f);
});

async function confirmDeleteTemplate() {
  const f = selectedFilename.value;
  deleteTplOpen.value = false;
  if (!f) return;
  const result = await remove(f);
  if (result.ok) {
    toast.success("workspace.templates.delete.success", [f.replace(/\.yaml$/, "")]);
  } else {
    toast.error("workspace.templates.delete.error", [result.message ?? "?"]);
  }
}

// ── Topbar menu ──────────────────────────────────────────────────────
setTopbarMenu(() => [
  {
    type: "group",
    id: "file",
    labelKey: "menu.file",
    items: [
      {
        id: "save",
        labelKey: "workspace.templates.save",
        disabled: !dirty.value,
        onClick: doSave,
      },
      {
        id: "reset",
        labelKey: "workspace.templates.reset",
        disabled: !dirty.value,
        onClick: doReset,
      },
      { type: "separator", id: "sep" },
      {
        id: "refresh",
        labelKey: "common.refresh",
        onClick: doRefresh,
      },
    ],
  },
  {
    type: "group",
    id: "template",
    labelKey: "menu.template",
    items: [
      {
        id: "create",
        labelKey: "menu.template.create",
        onClick: openCreate,
      },
      {
        id: "delete",
        labelKey: "menu.template.delete",
        disabled: !selectedFilename.value,
        onClick: openDeleteTemplate,
      },
    ],
  },
]);
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <span v-if="dirty" class="badge badge-warn">
        {{ t('workspace.templates.dirty_indicator') }}
      </span>
      <button
        class="tool-btn primary"
        :disabled="!draft"
        @click="openAddField"
      >
        + {{ t('workspace.templates.new_field') }}
      </button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.templates.sidebar_title') }}</h2>

      <p v-if="filenames.length === 0" class="muted small">
        {{ t('workspace.templates.empty') }}
      </p>

      <ul v-else class="template-list">
        <li
          v-for="f in filenames"
          :key="f"
          :class="['template-list-item', { active: f === selectedFilename }]"
          @click="selectedFilename = f"
        >
          <span class="template-display">{{ displayName(f) }}</span>
          <span class="template-meta">
            <span class="badge small template-filename">{{ f }}</span>
          </span>
        </li>
      </ul>
    </template>

    <template #main>
      <p v-if="!selectedTemplate || !draft" class="muted">
        {{ t('workspace.templates.unselected') }}
      </p>

      <template v-else>
        <div class="workspace-heading-row">
          <h1 class="workspace-heading">{{ draft.name || selectedFilename }}</h1>
          <span class="badge badge-accent">{{ selectedFilename }}</span>
          <span v-if="dirty" class="badge badge-warn">
            {{ t('workspace.templates.dirty_indicator') }}
          </span>
        </div>

        <FormSection :title="t('workspace.templates.setup.title')">
          <FormRow :label="t('workspace.templates.setup.template_name')">
            <TextField v-model="draft.name" />
          </FormRow>
          <FormRow :label="t('workspace.templates.setup.item_field')">
            <SelectField
              :model-value="draft.item_field || ''"
              @update:model-value="(v) => (draft && (draft.item_field = v))"
              :options="itemFieldSelectOptions"
            />
          </FormRow>
          <FormRow
            :label="t('workspace.templates.setup.template_code')"
            :description="t('workspace.templates.setup.template_code_help')"
          >
            <CodeEditor
              v-model="draft.markdown_template"
              lang="markdown"
              :height="120"
            />
          </FormRow>
          <FormRow :label="t('workspace.templates.setup.sidebar_expression')">
            <TextareaField v-model="draft.sidebar_expression" :rows="3" />
          </FormRow>
          <FormRow :label="t('workspace.templates.setup.enable_collection')">
            <SwitchField
              v-model="draft.enable_collection"
              :on-label="t('common.enabled')"
              :off-label="t('common.disabled')"
            />
          </FormRow>
        </FormSection>

        <FormSection :title="t('workspace.templates.fields.title')">
          <div class="fields-content">
          <p v-if="!draft.fields || draft.fields.length === 0" class="muted small">
            {{ t('workspace.templates.fields.empty') }}
          </p>
          <draggable
            v-else
            v-model="draft.fields"
            tag="ul"
            class="field-rows"
            handle=".field-drag-handle"
            :animation="150"
            ghost-class="field-row-ghost"
            chosen-class="field-row-chosen"
            drag-class="field-row-drag"
            item-key="key"
          >
            <template #item="{ element: f, index: i }">
              <li class="field-row" :data-type="f.type">
                <span class="field-drag-handle" aria-hidden="true">☰</span>
                <span class="field-row-label">{{ f.label || f.key || `(field ${i + 1})` }}</span>
                <span class="field-row-type">({{ (f.type || '').toUpperCase() }})</span>
                <span v-if="f.primary_key" class="badge badge-ok small">PRIMARY</span>
                <span class="field-row-spacer"></span>
                <div class="field-row-actions">
                  <button
                    type="button"
                    class="field-action-btn edit"
                    :disabled="f.type === 'loopstop'"
                    @click="openEdit(i)"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    class="field-action-btn delete"
                    :disabled="f.type === 'loopstop'"
                    @click="openDelete(i)"
                  >
                    Delete
                  </button>
                </div>
              </li>
            </template>
          </draggable>
          </div>
        </FormSection>
      </template>
    </template>
  </SplitPane>

  <!-- Create modal -->
  <Modal
    :open="createOpen"
    :title="t('workspace.templates.create.title')"
    @close="createOpen = false"
  >
    <label class="create-row">
      <span class="create-label">{{ t('workspace.templates.create.label') }}</span>
      <input
        class="field-input"
        v-model="createInput"
        :placeholder="t('workspace.templates.create.placeholder')"
        @keydown.enter="submitCreate"
      />
    </label>
    <p class="muted small create-help">
      {{ t('workspace.templates.create.help') }}
    </p>
    <p v-if="createError" class="form-error">{{ createError }}</p>

    <template #footer>
      <button class="tool-btn" type="button" @click="createOpen = false">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="submitCreate">
        {{ t('workspace.templates.new_template') }}
      </button>
    </template>
  </Modal>

  <!-- Field edit / add modal -->
  <FieldEditModal
    :open="editOpen"
    :field="editField"
    :is-new="editIsNew"
    @close="editOpen = false"
    @confirm="applyEdit"
  />

  <!-- Delete-field confirm -->
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

  <!-- Delete-template confirm -->
  <ConfirmDialog
    :open="deleteTplOpen"
    :title="t('workspace.templates.delete.title')"
    :message="t('workspace.templates.delete.confirm', [deleteTplName])"
    :confirm-label="t('workspace.profiles.action.delete')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="deleteTplOpen = false"
    @confirm="confirmDeleteTemplate"
  />
</template>

<style scoped>
/* Sidebar list — same visual language as the profile list. */
.template-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
}

.template-list-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: var(--space-2);
    border-radius: var(--radius-md);
    cursor: pointer;
    color: var(--color-text);
    background: transparent;
}

.template-list-item:hover { background: var(--list-hover-bg); }

.template-list-item.active {
    background: var(--list-active-bg);
    color: var(--list-active-fg);
}

.template-display { font-weight: 600; }

.template-meta {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-wrap: wrap;
    justify-content: flex-end;     /* badges float to bottom-right */
}

.template-filename {
    font-family: var(--font-mono);
    font-size: 11px;
}

/* Main panel — heading row keeps the title beside its meta pills. */
.workspace-heading-row {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    flex-wrap: wrap;
    margin-bottom: var(--space-3);
}

/* The .workspace-heading inside the row drops its own bottom margin
   so spacing is governed by the parent flex row + the global
   margin-bottom on .workspace-heading-row. */
.workspace-heading-row .workspace-heading {
    margin: 0;
}

/* Field Information section's content needs to span the FormSection's
   2-column grid so rows fill the panel width. */
.fields-content {
    grid-column: 1 / -1;
}


/* Field rows — Formidable's "label (TYPE) … Edit Delete" look. */
.field-rows {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
}

.field-row {
    /* Layout only — color/background/border live in
       styles/field-types.css so the per-type palette can win without
       fighting :scoped specificity. */
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 8px var(--space-3);
    border-radius: var(--radius-md);
}

.field-drag-handle {
    cursor: grab;
    user-select: none;
    font-size: 16px;
    line-height: 1;
    opacity: 0.85;
}

.field-drag-handle:active {
    cursor: grabbing;
}

.field-row-label {
    font-weight: 600;
    font-size: var(--font-size-md);
}

.field-row-type {
    font-family: var(--font-mono);
    font-size: 11px;
    letter-spacing: 0.04em;
    padding: 2px 8px;
    border-radius: 999px;
    line-height: 1.4;
    font-weight: 600;
}

.field-row-spacer {
    flex: 1 1 auto;
}

.field-row-actions {
    display: flex;
    gap: 6px;
}

.field-action-btn {
    appearance: none;
    border: 0;
    padding: 4px 12px;
    border-radius: var(--radius-md);
    font: inherit;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    color: #ffffff;
    line-height: 1.4;
}

.field-action-btn.edit {
    background: #f59e0b;            /* warning amber — Edit */
}
.field-action-btn.delete {
    background: #dc2626;            /* danger red — Delete */
}

.field-action-btn:hover:not(:disabled) {
    filter: brightness(1.08);
}

.field-action-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

/* Create modal */
.create-row {
    display: flex;
    flex-direction: column;
    gap: 6px;
}

.create-label {
    font-weight: 600;
    font-size: var(--font-size-sm);
}

.create-help {
    margin: var(--space-2) 0 0;
}
</style>
