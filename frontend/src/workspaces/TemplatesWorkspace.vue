<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import SplitPane from "../components/SplitPane.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import FieldEditModal from "../components/FieldEditModal.vue";
import GenerateTemplateDialog from "../components/GenerateTemplateDialog.vue";
import FieldScopeBadge from "../components/FieldScopeBadge.vue";
import TemplateListItem from "../components/TemplateListItem.vue";
import ExpressionBuilderModal from "../components/ExpressionBuilderModal.vue";
import CodeEditor from "../components/CodeEditor.vue";
import {
  Service as TemplateSvc,
  GeneratorOptions,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import {
  FormSection,
  FormRow,
  TextField,
  TextareaField,
  SelectField,
  SwitchField,
} from "../components/fields";
import { useTemplates, isValidTemplateFilename } from "../composables/useTemplates";
import { recomputeLevelScopes } from "../utils/fieldScopes";
import { useTemplateEditor } from "../composables/useTemplateEditor";
import { useRestartGate } from "../composables/useRestartGate";
import { useToast } from "../composables/useToast";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useConfig } from "../composables/useConfig";
import { watch } from "vue";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config, update: updateConfig } = useConfig();
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

// Two-way sync between the sidebar selection and config. Config is
// the single source of truth — Storage's dropdown writes there too,
// so any cross-workspace change propagates here live.
//
// Direction 1: config.selected_template → sidebar highlight. Fires
// whenever the config value or the loaded filenames list changes
// (need both — config can name a template that hasn't loaded yet
// during early boot).
watch(
  [() => config.value?.selected_template ?? "", filenames],
  ([want, list]) => {
    if (!list.length || !want) return;
    if (!list.includes(want)) return;
    if (selectedFilename.value !== want) selectedFilename.value = want;
  },
  { immediate: true },
);

// Direction 2: sidebar click → config. Skip when config already
// reflects this choice (avoids a redundant write triggered by the
// mirror watcher above).
watch(selectedFilename, (fn) => {
  if (!fn) return;
  if (config.value?.selected_template === fn) return;
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
    return;
  }
  if (result.reason === "validation") {
    // One toast per error — same shape the original Formidable used.
    // formatError already produced i18n {key, args} pairs.
    for (const err of result.errors) {
      toast.error(err.key, err.args);
    }
    return;
  }
  if (result.reason === "exception") {
    toast.error("workspace.templates.save_error", [result.message]);
    return;
  }
  // "no-draft" — guarded by the early return at top, but kept exhaustive.
  toast.error("workspace.templates.save_error", ["?"]);
}

function doReset() {
  reset();
}

// hasGuidField gates the Enable Collection switch — collection mode
// requires a record-level guid for the wiki/API resolver, so we don't
// let users flip the toggle on without one. Mirrors backend
// validation.collectionGuidError; without this gate, the user reaches
// "Save" only to be rejected after the fact.
//
// Asymmetric: when Collection is already ON we let the user toggle it
// OFF even without a guid (recovery path for templates that somehow
// got into the broken state — e.g. a guid field was removed manually).
const hasGuidField = computed(() => {
  const fields = draft.value?.fields ?? [];
  return fields.some((f: Field) => f.type === "guid");
});
const collectionToggleDisabled = computed(() => {
  return !hasGuidField.value && !draft.value?.enable_collection;
});

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

// ── Generate-template dialog ─────────────────────────────────────────
const generateOpen = ref(false);

async function applyGenerated(shape: string, opts: GeneratorOptions) {
  generateOpen.value = false;
  if (!draft.value) return;
  try {
    const out = await TemplateSvc.GenerateMarkdown(shape, opts, draft.value.fields ?? []);
    draft.value.markdown_template = out ?? "";
  } catch (err) {
    toast.error(t('workspace.templates.generate.error', [String(err)]));
  }
}

// ── Expression-builder dialog ────────────────────────────────────────
// Visual builder for sidebar_expression. The dialog is the only way
// to edit the source — the textarea is rendered read-only — so the
// shape stays predictable for the strict round-trip parser. On open
// the dialog tries to load the existing source; if parsing fails it
// emits "clear" and we wipe the textarea so the unparseable string
// can't survive a session.
const expressionBuilderOpen = ref(false);

function applyExpressionBuilder(source: string) {
  expressionBuilderOpen.value = false;
  if (!draft.value) return;
  draft.value.sidebar_expression = source;
}

function clearExpressionSource() {
  if (!draft.value) return;
  draft.value.sidebar_expression = "";
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

  <SplitPane :initial="sidebarWidth" :sidebar-split="true">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.templates.sidebar_title') }}</h2>

      <div class="sidebar-scroll">
        <p v-if="filenames.length === 0" class="muted small">
          {{ t('workspace.templates.empty') }}
        </p>

        <ul v-else class="template-list">
          <TemplateListItem
            v-for="f in filenames"
            :key="f"
            :filename="f"
            :display="displayName(f)"
            :active="f === selectedFilename"
            @pick="(name) => (selectedFilename = name)"
          />
        </ul>
      </div>
    </template>

    <template #main>
      <p
        v-if="!selectedTemplate || !draft"
        class="workspace-empty"
        v-html="t('workspace.templates.unselected')"
      ></p>

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
            <div
              v-if="!draft.markdown_template || !draft.markdown_template.trim()"
              class="generate-template-row"
            >
              <button
                class="tool-btn"
                type="button"
                @click="generateOpen = true"
              >
                {{ t('workspace.templates.generate.button') }}
              </button>
            </div>
          </FormRow>
          <FormRow :label="t('workspace.templates.setup.sidebar_expression')">
            <TextareaField
              v-model="draft.sidebar_expression"
              :rows="3"
              :readonly="true"
            />
            <div class="expression-builder-row">
              <button
                class="tool-btn"
                type="button"
                @click="expressionBuilderOpen = true"
              >
                {{ t('workspace.templates.expression_builder.button') }}
              </button>
            </div>
          </FormRow>
          <FormRow :label="t('workspace.templates.setup.enable_collection')">
            <div class="collection-toggle">
              <SwitchField
                v-model="draft.enable_collection"
                :on-label="t('common.enabled')"
                :off-label="t('common.disabled')"
                :disabled="collectionToggleDisabled"
              />
              <p
                v-if="collectionToggleDisabled"
                class="muted small"
              >
                {{ t('workspace.templates.setup.enable_collection_needs_guid') }}
              </p>
            </div>
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
            handle=".dnd-handle"
            :animation="150"
            ghost-class="dnd-ghost"
            chosen-class="dnd-chosen"
            drag-class="dnd-drag"
            item-key="key"
            @end="recomputeLevelScopes(draft.fields)"
          >
            <template #item="{ element: f, index: i }">
              <li class="field-row" :data-type="f.type">
                <span class="dnd-handle" aria-hidden="true">☰</span>
                <span class="field-row-label">{{ f.label || f.key || `(field ${i + 1})` }}</span>
                <span class="field-row-type">({{ (f.type || '').toUpperCase() }})</span>
                <span v-if="f.primary_key" class="badge badge-ok small">PRIMARY</span>
                <span class="field-row-spacer"></span>
                <div class="field-row-actions">
                  <FieldScopeBadge :level="f.level_scope" />
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
    <label class="dialog-row">
      <span class="dialog-row-label">{{ t('workspace.templates.create.label') }}</span>
      <input
        class="field-input"
        v-model="createInput"
        :placeholder="t('workspace.templates.create.placeholder')"
        @keydown.enter="submitCreate"
      />
    </label>
    <p class="muted small dialog-row-help">
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

  <!-- Generate-template dialog: shape + sub-options -->
  <GenerateTemplateDialog
    :open="generateOpen"
    @cancel="generateOpen = false"
    @confirm="(shape, opts) => applyGenerated(shape, opts)"
  />

  <!-- Expression builder dialog: visual builder for sidebar_expression -->
  <ExpressionBuilderModal
    v-if="draft"
    :open="expressionBuilderOpen"
    :fields="draft.fields ?? []"
    :initial="draft.sidebar_expression"
    @close="expressionBuilderOpen = false"
    @apply="applyExpressionBuilder"
    @clear="clearExpressionSource"
  />
</template>

<style scoped>
.generate-template-row {
  margin-top: 0.5rem;
  display: flex;
  justify-content: flex-start;
}
</style>

