<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import SplitPane from "../components/SplitPane.vue";
import Modal from "../components/Modal.vue";
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

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const toast = useToast();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const {
  filenames,
  cache,
  selectedFilename,
  selectedTemplate,
  refresh,
  create,
} = useTemplates();

const { draft, dirty, itemFieldOptions, save, reset } = useTemplateEditor();

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
]);
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <span v-if="dirty" class="badge badge-warn">
        {{ t('workspace.templates.dirty_indicator') }}
      </span>
      <button class="tool-btn primary" @click="openCreate">
        + {{ t('workspace.templates.new_template') }}
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
              :height="260"
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
                    title="Edit field — wired in next step"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    class="field-action-btn delete"
                    :disabled="f.type === 'loopstop'"
                    title="Delete field — wired in next step"
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
