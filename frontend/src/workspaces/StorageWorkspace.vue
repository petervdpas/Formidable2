<script setup lang="ts">
import { computed, provide, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import RightSlideout from "../components/RightSlideout.vue";
import { FormSection, SelectField, TextField, SwitchField } from "../components/fields";
import FormLoopFields from "../components/form-fields/FormLoopFields.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useTemplates } from "../composables/useTemplates";
import { useFormView } from "../composables/useFormView";
import { useConfig } from "../composables/useConfig";
import { useToast } from "../composables/useToast";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { Service as FormSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import type { FormSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config, update: updateConfig } = useConfig();
const { filenames: templateFilenames, cache: templateCache } = useTemplates();
const { view, draft, dirty, open, close, save, reset, remove } = useFormView();
const toast = useToast();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

// Active template's filename — provided downward so per-type field
// components that need it (image saves into <storage>/<tplName>/images/,
// for example) can inject without prop-drilling through the renderer.
const currentTemplateFilename = computed(
  () => draft.value?.template?.filename ?? "",
);
provide("templateFilename", currentTemplateFilename);

// ── Active template selection ────────────────────────────────────────
// Read-only computed off config — onTemplateChange below writes back
// when the dropdown fires. Switching templates also clears the
// selected datafile so we don't try to open a form whose schema no
// longer matches.
const selectedTemplate = computed<string>(
  () => config.value?.selected_template ?? "",
);

function onTemplateChange(filename: string) {
  if (filename === selectedTemplate.value) return;
  void updateConfig({
    selected_template: filename,
    selected_data_file: "",
  });
}

const templateOptions = computed(() =>
  templateFilenames.value.map((f) => {
    const tpl = templateCache.value.get(f);
    return { value: f, label: tpl?.name?.trim() || f.replace(/\.yaml$/, "") };
  }),
);

// ── Form list (sidebar) ──────────────────────────────────────────────
const summaries = ref<FormSummary[]>([]);
const listError = ref("");

async function refreshList() {
  if (!selectedTemplate.value) {
    summaries.value = [];
    return;
  }
  listError.value = "";
  try {
    await FormSvc.EnsureFormDir(selectedTemplate.value);
    summaries.value = await FormSvc.ListForms(selectedTemplate.value);
  } catch (err) {
    listError.value = String(err);
    summaries.value = [];
  }
}

// Template change → refresh the sidebar list. The combined watcher
// below owns the open/close lifecycle of the form view; we don't
// touch it here, otherwise the close() races with the open() that
// the combined watcher dispatches on initial mount with a persisted
// (template, datafile) pair.
watch(selectedTemplate, async () => {
  await refreshList();
}, { immediate: true });

// ── Selected datafile (persisted in config) ──────────────────────────
const selectedDataFile = computed<string>({
  get: () => config.value?.selected_data_file ?? "",
  set: (v) => { void updateConfig({ selected_data_file: v }); },
});

watch(
  [selectedTemplate, selectedDataFile],
  async ([tpl, df], oldVals) => {
    if (!tpl || !df) {
      close();
      return;
    }
    // If the template changed, drop the prior form (different schema)
    // before loading the new one so we never render stale fields.
    const prevTpl = oldVals?.[0];
    if (prevTpl && prevTpl !== tpl) close();
    await open(tpl, df);
  },
  { immediate: true },
);

function pickForm(filename: string) {
  selectedDataFile.value = filename;
}

// ── Sidebar filters (chrome only for v1 — patch behaviour later) ────
const showAll = ref(false);
const tagFilter = ref("");
const visibleSummaries = computed(() => {
  // Marked-only filter is a no-op until storage exposes flagged in the
  // summary; the toggle stays so the layout matches the original.
  return summaries.value;
});

// ── New Entry dialog ─────────────────────────────────────────────────
const newOpen = ref(false);
const newName = ref("");
const newError = ref("");
const newAppendDate = ref(false);

function openNew() {
  if (!selectedTemplate.value) return;
  newName.value = "";
  newError.value = "";
  newAppendDate.value = false;
  newOpen.value = true;
}

// "YYYYMMDD" suffix from today's date (local time — matches the
// original Formidable, which also uses local-zone date for filenames).
function todayYYYYMMDD(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}${m}${day}`;
}

async function submitNew() {
  const raw = newName.value.trim();
  if (!raw) {
    newError.value = t("workspace.storage.new.invalid");
    return;
  }
  const stem = raw.endsWith(".meta.json")
    ? raw.slice(0, -".meta.json".length)
    : raw;
  const dated = newAppendDate.value ? `${stem}-${todayYYYYMMDD()}` : stem;
  const filename = `${dated}.meta.json`;
  if (!/^[a-zA-Z0-9._-]+\.meta\.json$/.test(filename)) {
    newError.value = t("workspace.storage.new.invalid_chars");
    return;
  }
  if (summaries.value.some((s) => s.filename === filename)) {
    newError.value = t("workspace.storage.new.exists");
    return;
  }
  // Open an unsaved view, set selection — persist happens on first Save.
  selectedDataFile.value = filename;
  await open(selectedTemplate.value, filename);
  newOpen.value = false;
  toast.success("workspace.storage.new.opened", [filename]);
}

// ── Save / Reset / Delete ────────────────────────────────────────────
async function doSave() {
  if (!draft.value) return;
  const result = await save();
  if (result.ok) {
    toast.success("workspace.storage.save.success", [draft.value?.datafile ?? "?"]);
    await refreshList();
  } else {
    toast.error("workspace.storage.save.error", [result.message ?? "?"]);
  }
}

const deleteOpen = ref(false);
function askDelete() {
  if (view.value?.saved) deleteOpen.value = true;
}
async function confirmDelete() {
  deleteOpen.value = false;
  const filename = view.value?.datafile ?? "";
  const result = await remove();
  if (result.ok) {
    toast.success("workspace.storage.delete.success", [filename]);
    selectedDataFile.value = "";
    await refreshList();
  } else {
    toast.error("workspace.storage.delete.error", [result.message ?? "?"]);
  }
}

// ── Preview slideouts ────────────────────────────────────────────────
const mdOpen = ref(false);
const htmlOpen = ref(false);

// ── Topbar menu ──────────────────────────────────────────────────────
setTopbarMenu(() => [
  {
    type: "group",
    id: "file",
    labelKey: "menu.file",
    items: [
      {
        id: "save",
        labelKey: "workspace.storage.save",
        disabled: !dirty.value,
        onClick: doSave,
      },
      {
        id: "reset",
        labelKey: "workspace.storage.reset",
        disabled: !dirty.value,
        onClick: reset,
      },
      { type: "separator", id: "sep" },
      {
        id: "refresh",
        labelKey: "common.refresh",
        onClick: refreshList,
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
        {{ t('workspace.storage.dirty_indicator') }}
      </span>
      <button
        v-if="view"
        class="tool-btn primary"
        :disabled="!dirty"
        @click="doSave"
      >
        {{ t('workspace.storage.save') }}
      </button>
      <button
        v-if="view"
        class="tool-btn danger"
        :disabled="!view.saved"
        @click="askDelete"
      >
        {{ t('workspace.storage.delete') }}
      </button>
      <button
        class="tool-btn primary"
        :disabled="!selectedTemplate"
        @click="openNew"
      >
        + {{ t('workspace.storage.new_entry') }}
      </button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.storage.sidebar_title') }}</h2>

      <div class="sidebar-section">
        <label class="sidebar-label">{{ t('workspace.storage.template_picker') }}</label>
        <SelectField
          :model-value="selectedTemplate"
          @update:model-value="onTemplateChange"
          :options="templateOptions"
        />
      </div>

      <div class="sidebar-section">
        <div class="sidebar-section-head">
          <span class="sidebar-label">{{ t('workspace.storage.forms_heading') }}</span>
          <span class="muted small">{{ summaries.length }}</span>
        </div>

        <div class="sidebar-toolbar">
          <SwitchField
            v-model="showAll"
            :on-label="t('workspace.storage.show_all')"
            :off-label="t('workspace.storage.show_marked')"
          />
        </div>

        <TextField
          v-model="tagFilter"
          :placeholder="t('workspace.storage.tag_filter_placeholder')"
        />
      </div>

      <p v-if="!selectedTemplate" class="muted small">
        {{ t('workspace.storage.no_template_selected') }}
      </p>
      <p v-else-if="listError" class="form-error small">{{ listError }}</p>
      <p v-else-if="visibleSummaries.length === 0" class="muted small">
        {{ t('workspace.storage.empty') }}
      </p>

      <ul v-else class="form-list">
        <li
          v-for="s in visibleSummaries"
          :key="s.filename"
          :class="['sidebar-row', 'sidebar-row--stack', { active: s.filename === selectedDataFile }]"
          @click="pickForm(s.filename)"
        >
          <span class="form-list-title">{{ s.title || s.filename }}</span>
          <span class="form-list-filename">{{ s.filename }}</span>
        </li>
      </ul>
    </template>

    <template #main>
      <p v-if="!selectedTemplate" class="workspace-empty">
        {{ t('workspace.storage.placeholder_main') }}
      </p>
      <p v-else-if="!view || !draft" class="workspace-empty">
        {{ t('workspace.storage.unselected') }}
      </p>

      <template v-else>
        <!-- Meta scaffold — full polish patched in later. -->
        <FormSection>
          <div class="meta-grid">
            <div class="meta-row" v-if="draft.datafile">
              <span class="meta-key">{{ t('workspace.storage.meta.filename') }}</span>
              <span class="meta-value mono">{{ draft.datafile }}</span>
            </div>
            <div class="meta-row" v-if="draft.meta?.id">
              <span class="meta-key">{{ t('workspace.storage.meta.id') }}</span>
              <span class="meta-value mono">{{ draft.meta.id }}</span>
            </div>
            <div class="meta-row" v-if="draft.meta?.tags?.length">
              <span class="meta-key">{{ t('workspace.storage.meta.tags') }}</span>
              <span class="meta-value">{{ draft.meta.tags.join(', ') }}</span>
            </div>
            <div class="meta-row" v-if="draft.meta?.author_name">
              <span class="meta-key">{{ t('workspace.storage.meta.author') }}</span>
              <span class="meta-value">{{ draft.meta.author_name }}</span>
            </div>
            <div class="meta-row" v-if="draft.meta?.created">
              <span class="meta-key">{{ t('workspace.storage.meta.created') }}</span>
              <span class="meta-value mono small">{{ draft.meta.created }}</span>
            </div>
            <div class="meta-row" v-if="draft.meta?.updated">
              <span class="meta-key">{{ t('workspace.storage.meta.updated') }}</span>
              <span class="meta-value mono small">{{ draft.meta.updated }}</span>
            </div>
          </div>

        </FormSection>

        <!-- Plain wrapper — NOT FormSection — so each row spans the
             full panel width. FormSection's own grid would tile rows
             into its label/value columns and pair them up. -->
        <div v-if="draft.template" class="form-fields">
          <FormLoopFields
            :fields="draft.template.fields"
            :start-offset="0"
            :values="draft.values"
            :loop-groups="draft.loop_groups"
          />
        </div>
      </template>
    </template>
  </SplitPane>

  <!-- Right-edge preview slideouts: teleported to #app-main so they
       span the entire workspace width (sidebar + main) up to the ribbon. -->
  <RightSlideout
    v-model:open="mdOpen"
    :title="t('workspace.storage.preview.markdown')"
    :handle-label="t('workspace.storage.preview.markdown_handle')"
    offset-top="var(--space-3)"
  />
  <RightSlideout
    v-model:open="htmlOpen"
    :title="t('workspace.storage.preview.html')"
    :handle-label="t('workspace.storage.preview.html_handle')"
    offset-top="calc(var(--space-3) + var(--right-slideout-handle-h) + 1px)"
  />

  <!-- New entry dialog -->
  <Modal
    :open="newOpen"
    :title="t('workspace.storage.new.title')"
    @close="newOpen = false"
  >
    <div class="dialog-grid">
      <label class="dialog-grid-label" for="new-entry-name">
        {{ t('workspace.storage.new.label') }}
      </label>
      <input
        id="new-entry-name"
        class="field-input"
        v-model="newName"
        :placeholder="t('workspace.storage.new.placeholder')"
        @keydown.enter="submitNew"
      />

      <span class="dialog-grid-label">
        {{ t('workspace.storage.new.append_date') }}
      </span>
      <SwitchField v-model="newAppendDate" />
    </div>
    <p v-if="newError" class="form-error">{{ newError }}</p>

    <template #footer>
      <button class="tool-btn" type="button" @click="newOpen = false">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="submitNew">
        {{ t('workspace.storage.new_entry') }}
      </button>
    </template>
  </Modal>

  <!-- Delete confirm -->
  <ConfirmDialog
    :open="deleteOpen"
    :title="t('workspace.storage.delete.title')"
    :message="t('workspace.storage.delete.confirm', [view?.datafile ?? ''])"
    :confirm-label="t('workspace.storage.delete.button')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="deleteOpen = false"
    @confirm="confirmDelete"
  />
</template>

