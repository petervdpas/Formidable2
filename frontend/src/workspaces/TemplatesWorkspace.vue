<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import SplitPane from "../components/SplitPane.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import FieldEditModal from "../components/FieldEditModal.vue";
import GenerateTemplateDialog from "../components/GenerateTemplateDialog.vue";
import CleanupStorageDialog from "../components/CleanupStorageDialog.vue";
import InjectPDFFrontmatterDialog from "../components/InjectPDFFrontmatterDialog.vue";
import FieldScopeBadge from "../components/FieldScopeBadge.vue";
import TemplateListItem from "../components/TemplateListItem.vue";
import ExpressionBuilderModal from "../components/ExpressionBuilderModal.vue";
import FlagDefinitionsModal from "../components/FlagDefinitionsModal.vue";
import { MAX_FLAG_DEFINITIONS } from "../utils/flagColors";
import CodeEditor from "../components/CodeEditor.vue";
import Tabs from "../components/Tabs.vue";
import {
  Service as TemplateSvc,
  GeneratorOptions,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as ExpressionSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { Service as SystemSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { Service as PdfSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf";
import type { FieldRef } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/builder";
import { backendErrMessage } from "../utils/backendError";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
  TextareaField,
  SelectField,
} from "../components/fields";
import { useTemplates, isValidTemplateFilename } from "../composables/useTemplates";
import { recomputeLevelScopes } from "../utils/fieldScopes";
import { useTemplateEditor } from "../composables/useTemplateEditor";
import { useRestartGate } from "../composables/useRestartGate";
import { useToast } from "../composables/useToast";
import { useStatusBar } from "../composables/useStatusBar";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useConfig } from "../composables/useConfig";
import { watch } from "vue";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config, update: updateConfig } = useConfig();
const toast = useToast();
const statusBar = useStatusBar();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

// Sidebar honors per-profile EnabledTemplates curation — same filtered
// list the storage picker uses. The Settings → Templates panel is the
// only entry point that sees ALL templates (so the user can enable a
// disabled one). The editor follows the curation.
const {
  enabledFilenames: filenames,
  cache,
  selectedFilename,
  selectedTemplate,
  refresh,
  refreshOne,
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
  if (config.value?.selected_template !== fn) {
    void updateConfig({ selected_template: fn });
  }
  statusBar.setSelected(fn);
});

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
    const fn = selectedFilename.value!;
    toast.success("workspace.templates.save_success", [fn]);
    statusBar.setSaved(fn);
    // Re-read just the saved template into the cache. The other rows
    // are untouched, so the rest of the list (and its scroll position)
    // stays stable — Vue propagates the new entry to the matching
    // TemplateListItem via its :template prop.
    await refreshOne(fn);
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
  toast.success("workspace.templates.create.success", [name]);
  statusBar.setCreated(name);
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

// ── Cleanup-storage dialog (Utilities → Cleanup Storage) ────────────
const cleanupOpen = ref(false);

// ── PDF frontmatter utilities (Utilities → Inject / Migrate PDF FM) ──
// Inject prepends the canonical picoloom scaffold to markdown_template
// (refuses if a `---` block is already there). Migrate parses an
// existing eisvogel-style frontmatter and rewrites it into picoloom
// shape, surfacing a preview modal so the user can see the mappings,
// preserved-as-legacy keys, and warnings before applying. Both
// operations only edit the in-memory draft; the user still has to
// click Save on the template to persist.
// Migrate keeps its preview-and-confirm modal (the diff IS the point).
// Inject moves to the form-based InjectPDFFrontmatterDialog wizard —
// the YAML-preview UX was hostile per the user's feedback. The two
// flows still share the apply step (write scaffold to draft).
const pdfFmDialogOpen = ref(false);
const pdfFmProposed = ref(""); // migrate preview content
const pdfFmMigration = ref<{
  mappings: { from: string; to: string }[];
  preserved: string[];
  warnings: string[];
  had_frontmatter: boolean;
} | null>(null);

const pdfInjectOpen = ref(false);

function openPdfFmInject() {
  if (!draft.value) return;
  // Refuse if frontmatter already exists — direct user to Migrate.
  const md = draft.value.markdown_template ?? "";
  if (/^---\s*\n/.test(md)) {
    toast.error("workspace.templates.pdf_fm.inject_refused");
    return;
  }
  pdfInjectOpen.value = true;
}

function applyInjectedScaffold(scaffold: string) {
  if (!draft.value) return;
  const md = draft.value.markdown_template ?? "";
  draft.value.markdown_template = scaffold + md;
  pdfInjectOpen.value = false;
  toast.success("workspace.templates.pdf_fm.inject_applied");
}

async function openPdfFmMigrate() {
  if (!draft.value) return;
  try {
    const result = await PdfSvc.MigrateFrontmatter(draft.value.markdown_template ?? "");
    if (!result?.had_frontmatter) {
      toast.info("workspace.templates.pdf_fm.no_frontmatter");
      return;
    }
    pdfFmProposed.value = result.markdown ?? "";
    pdfFmMigration.value = {
      mappings: result.mappings ?? [],
      preserved: result.preserved ?? [],
      warnings: result.warnings ?? [],
      had_frontmatter: result.had_frontmatter,
    };
    pdfFmDialogOpen.value = true;
  } catch (e) {
    toast.error("workspace.templates.pdf_fm.migrate_failed", [backendErrMessage(e)]);
  }
}

function applyMigratedPdfFm() {
  if (!draft.value) return;
  draft.value.markdown_template = pdfFmProposed.value;
  pdfFmDialogOpen.value = false;
  toast.success("workspace.templates.pdf_fm.migrate_applied");
}

// ── Utilities → Open Folder actions ─────────────────────────────────
// Both delegate to System.OpenExternal which routes through xdg-open /
// open / rundll32 depending on platform. Templates folder is one
// service call; storage folder needs the current template's filename.
async function openTemplateFolder() {
  try {
    const path = await TemplateSvc.TemplatesDir();
    if (!path) return;
    await SystemSvc.OpenExternal(path);
  } catch (e) {
    toast.error("workspace.templates.open_folder.error", [backendErrMessage(e)]);
  }
}

async function openStorageFolder() {
  if (!selectedFilename.value) return;
  try {
    const path = await StorageSvc.TemplateStorageDir(selectedFilename.value);
    if (!path) return;
    await SystemSvc.OpenExternal(path);
  } catch (e) {
    toast.error("workspace.templates.open_folder.error", [backendErrMessage(e)]);
  }
}

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

// Inline "Convert" affordance next to the Builder button. Shows only
// when the current sidebar_expression is non-empty AND fails the
// strict builder Parse — i.e. it's a legacy shape (array-wrapped
// ternary, old `|` pipe form, bare identifiers, etc.) that the
// builder dialog can't load. Clicking Convert pipes the source
// through the backend best-effort migrator and writes the canonical
// result back into draft.sidebar_expression. The Builder button can
// then load it normally.
const expressionParseable = ref(true);

function expressionFieldRefs(): FieldRef[] {
  const fs = draft.value?.fields ?? [];
  return fs
    .filter((f) => {
      if (!f.expression_item) return false;
      const tt = (f.type || "").toLowerCase();
      return tt !== "loopstart" && tt !== "loopstop" && tt !== "looper";
    })
    .map((f) => ({
      key: f.key,
      type: f.type || "",
      options: ((f.options ?? []) as any[]).map((o) => ({
        value: String(o?.value ?? ""),
        label: String(o?.label ?? o?.value ?? ""),
      })),
    }));
}

async function recheckExpressionParseable() {
  const src = (draft.value?.sidebar_expression ?? "").trim();
  if (!src) {
    expressionParseable.value = true;
    return;
  }
  try {
    await ExpressionSvc.BuilderParse(src, expressionFieldRefs());
    expressionParseable.value = true;
  } catch {
    expressionParseable.value = false;
  }
}

async function onInlineConvert() {
  if (!draft.value) return;
  const src = (draft.value.sidebar_expression ?? "").trim();
  if (!src) return;
  try {
    const migrated = await ExpressionSvc.BuilderConvert(src, expressionFieldRefs());
    draft.value.sidebar_expression = migrated;
    toast.success("workspace.templates.expression_builder.convert_succeeded");
  } catch (err) {
    toast.error("workspace.templates.expression_builder.convert_failed");
    const detail = backendErrMessage(err);
    if (detail) toast.error(detail, undefined, { duration: 8000 });
  }
}

watch(
  () => draft.value?.sidebar_expression,
  () => {
    void recheckExpressionParseable();
  },
  { immediate: true },
);

// ── Setup-info tabs (Template Code / Sidebar Expression / Flag Defs) ─
const setupTab = ref<"code" | "expression" | "flags">("code");
const setupTabItems = computed(() => [
  { id: "code", label: t("workspace.templates.setup.template_code") },
  { id: "expression", label: t("workspace.templates.setup.sidebar_expression") },
  { id: "flags", label: t("workspace.templates.setup.flag_definitions") },
]);

// ── Flag-definitions builder dialog ──────────────────────────────────
const flagBuilderOpen = ref(false);

function applyFlagDefinitions(defs: import("../../bindings/github.com/petervdpas/formidable2/internal/modules/template").FlagDefinition[]) {
  if (!draft.value) return;
  draft.value.flag_definitions = defs;
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
  const cached = cache.value.get(f);
  if (cached?.name && cached.name.trim()) return cached.name;
  return f.replace(/\.yaml$/, "");
});

async function confirmDeleteTemplate() {
  const f = selectedFilename.value;
  deleteTplOpen.value = false;
  if (!f) return;
  const result = await remove(f);
  if (result.ok) {
    toast.success("workspace.templates.delete.success", [f]);
    statusBar.setDeleted(f);
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
    alwaysEnabled: true,
    items: [
      {
        id: "save",
        labelKey: "workspace.templates.save",
        combo: "Mod+S",
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
      { type: "separator", id: "sep-folders" },
      {
        id: "openTemplateFolder",
        labelKey: "menu.file.openTemplateFolder",
        onClick: openTemplateFolder,
      },
      {
        id: "openStorageFolder",
        labelKey: "menu.file.openStorageFolder",
        disabled: !selectedFilename.value,
        onClick: openStorageFolder,
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
        combo: "Mod+N",
        onClick: openCreate,
      },
      {
        id: "delete",
        labelKey: "menu.template.delete",
        combo: "Mod+D",
        disabled: !selectedFilename.value,
        onClick: openDeleteTemplate,
      },
    ],
  },
  {
    type: "group",
    id: "utilities",
    labelKey: "menu.utilities",
    items: [
      {
        id: "cleanupStorage",
        labelKey: "menu.utilities.cleanupStorage",
        disabled: !selectedFilename.value,
        onClick: () => { cleanupOpen.value = true; },
      },
      { type: "separator", id: "utils-sep-pdf" },
      {
        id: "injectPdfFrontmatter",
        labelKey: "menu.utilities.inject_pdf_frontmatter",
        disabled: !draft.value,
        onClick: openPdfFmInject,
      },
      {
        id: "migratePdfFrontmatter",
        labelKey: "menu.utilities.migrate_pdf_frontmatter",
        disabled: !draft.value,
        onClick: openPdfFmMigrate,
      },
    ],
  },
]);
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <Badge v-if="dirty" variant="warn">
        {{ t('workspace.templates.dirty_indicator') }}
      </Badge>
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
            :template="cache.get(f) ?? null"
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
          <Badge variant="accent">{{ selectedFilename }}</Badge>
          <Badge v-if="dirty" variant="warn">
            {{ t('workspace.templates.dirty_indicator') }}
          </Badge>
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
          <div class="setup-tabs-block">
            <Tabs v-model="setupTab" :items="setupTabItems">
              <template #code>
                <div class="setup-tab-pane">
                  <CodeEditor
                    v-model="draft.markdown_template"
                    lang="markdown"
                    :height="180"
                    :title="`${draft.name || selectedFilename} • ${t('workspace.templates.setup.template_code')}`"
                  />
                  <p class="muted small setup-tab-help">
                    {{ t('workspace.templates.setup.template_code_help') }}
                  </p>
                  <div
                    v-if="!draft.markdown_template || !draft.markdown_template.trim()"
                    class="setup-tab-actions"
                  >
                    <button
                      class="tool-btn"
                      type="button"
                      @click="generateOpen = true"
                    >
                      {{ t('workspace.templates.generate.button') }}
                    </button>
                  </div>
                </div>
              </template>

              <template #expression>
                <div class="setup-tab-pane">
                  <TextareaField
                    v-model="draft.sidebar_expression"
                    :rows="6"
                    :readonly="true"
                  />
                  <div class="setup-tab-actions">
                    <button
                      class="tool-btn"
                      type="button"
                      @click="expressionBuilderOpen = true"
                    >
                      {{ t('workspace.templates.expression_builder.button') }}
                    </button>
                    <button
                      v-if="!expressionParseable"
                      class="tool-btn primary"
                      type="button"
                      :title="t('workspace.templates.expression_builder.convert_title')"
                      @click="onInlineConvert"
                    >
                      {{ t('workspace.templates.expression_builder.convert') }}
                    </button>
                  </div>
                </div>
              </template>

              <template #flags>
                <div class="setup-tab-pane">
                  <p
                    v-if="!draft.flag_definitions || draft.flag_definitions.length === 0"
                    class="muted small"
                  >
                    {{ t('workspace.templates.flag_definitions.summary_empty') }}
                  </p>
                  <div v-else class="flag-definitions-summary">
                    <span
                      v-for="d in draft.flag_definitions"
                      :key="d.label"
                      class="flag-definitions-chip"
                      :class="`expr-bg-${d.color}`"
                      :title="`${d.label} (${d.color})`"
                    >{{ d.label }}</span>
                    <span class="muted small">
                      {{ t('workspace.templates.flag_definitions.summary',
                           [draft.flag_definitions.length, MAX_FLAG_DEFINITIONS]) }}
                    </span>
                  </div>
                  <div class="setup-tab-actions">
                    <button
                      class="tool-btn"
                      type="button"
                      @click="flagBuilderOpen = true"
                    >{{ t('workspace.templates.flag_definitions.builder_button') }}</button>
                  </div>
                </div>
              </template>
            </Tabs>
          </div>

          <FormSwitchRow
            v-model="draft.enable_collection"
            :label="t('workspace.templates.setup.enable_collection')"
            :description="collectionToggleDisabled
              ? t('workspace.templates.setup.enable_collection_needs_guid')
              : ''"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
            :disabled="collectionToggleDisabled"
          />
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
                <Badge v-if="f.primary_key" variant="ok" class="small">PRIMARY</Badge>
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

  <!-- Cleanup Storage dialog: analyzes the selected template's forms -->
  <CleanupStorageDialog
    :open="cleanupOpen"
    :template-filename="selectedFilename ?? ''"
    :template-label="selectedTemplate?.name"
    @close="cleanupOpen = false"
  />

  <!-- Inject PDF frontmatter — form-based wizard (toggles + dropdowns). -->
  <InjectPDFFrontmatterDialog
    :open="pdfInjectOpen"
    :template-name="selectedTemplate?.name"
    @cancel="pdfInjectOpen = false"
    @apply="applyInjectedScaffold"
  />

  <!-- Migrate PDF frontmatter — preview the eisvogel→picoloom rewrite
       alongside a summary of mapped/preserved/warnings. Diff IS the UX
       so this stays as a read-only preview modal. -->
  <Modal
    :open="pdfFmDialogOpen"
    :title="t('workspace.templates.pdf_fm.title_migrate')"
    width="820px"
    @close="pdfFmDialogOpen = false"
  >
    <p class="muted small">
      {{ t('workspace.templates.pdf_fm.intro_migrate') }}
    </p>

    <div v-if="pdfFmMigration" class="pdf-fm-summary">
      <div v-if="pdfFmMigration.mappings.length > 0" class="pdf-fm-summary-block">
        <h5>{{ t('workspace.templates.pdf_fm.summary.mappings') }}</h5>
        <ul>
          <li v-for="m in pdfFmMigration.mappings" :key="m.from + '→' + m.to">
            <code>{{ m.from }}</code> → <code>{{ m.to }}</code>
          </li>
        </ul>
      </div>
      <div v-if="pdfFmMigration.preserved.length > 0" class="pdf-fm-summary-block">
        <h5>{{ t('workspace.templates.pdf_fm.summary.preserved') }}</h5>
        <ul>
          <li v-for="k in pdfFmMigration.preserved" :key="k">
            <code>{{ k }}</code>
          </li>
        </ul>
      </div>
      <div v-if="pdfFmMigration.warnings.length > 0" class="pdf-fm-summary-block pdf-fm-summary-warn">
        <h5>{{ t('workspace.templates.pdf_fm.summary.warnings') }}</h5>
        <ul>
          <li v-for="(w, i) in pdfFmMigration.warnings" :key="i">{{ w }}</li>
        </ul>
      </div>
    </div>

    <CodeEditor
      :model-value="pdfFmProposed"
      lang="markdown"
      :readonly="true"
      :height="380"
      :title="t('workspace.templates.pdf_fm.title_migrate')"
    />

    <template #footer>
      <button class="tool-btn" type="button" @click="pdfFmDialogOpen = false">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="applyMigratedPdfFm">
        {{ t('workspace.templates.pdf_fm.action.apply') }}
      </button>
    </template>
  </Modal>

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

  <!-- Flag-definitions builder: edits draft.flag_definitions -->
  <FlagDefinitionsModal
    v-if="draft"
    :open="flagBuilderOpen"
    :initial="draft.flag_definitions ?? []"
    @close="flagBuilderOpen = false"
    @apply="applyFlagDefinitions"
  />
</template>

<style scoped>
.generate-template-row {
  margin-top: 0.5rem;
  display: flex;
  justify-content: flex-start;
}
</style>

