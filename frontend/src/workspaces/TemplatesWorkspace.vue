<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import TemplateFieldsSection from "../components/TemplateFieldsSection.vue";
import GenerateTemplateDialog from "../components/GenerateTemplateDialog.vue";
import CleanupStorageDialog from "../components/CleanupStorageDialog.vue";
import InjectPDFFrontmatterDialog from "../components/InjectPDFFrontmatterDialog.vue";
import TemplateListItem from "../components/TemplateListItem.vue";
import TemplateCodeTab from "../components/TemplateCodeTab.vue";
import TemplateExpressionTab from "../components/TemplateExpressionTab.vue";
import TemplateFacetsTab from "../components/TemplateFacetsTab.vue";
import TemplateFormulasTab from "../components/TemplateFormulasTab.vue";
import TemplateStatisticsTab from "../components/TemplateStatisticsTab.vue";
import Tabs from "../components/Tabs.vue";
import {
  Service as TemplateSvc,
  GeneratorOptions,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { Service as SystemSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { Service as PdfSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf";
import { Service as IndexSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/index";
import { Service as RelationSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation";
import TemplateRelationsTab from "../components/TemplateRelationsTab.vue";
import { backendErrMessage } from "../utils/backendError";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
  FieldSelector,
} from "../components/fields";
import { useTemplates } from "../composables/useTemplates";
import { useTemplateCreate } from "../composables/useTemplateCreate";
import { useTemplateSelection } from "../composables/useTemplateSelection";
import { useListKeyNav } from "../composables/useListKeyNav";
import { useTemplateEditor } from "../composables/useTemplateEditor";
import { useRestartGate } from "../composables/useRestartGate";
import { useToast } from "../composables/useToast";
import { useStatusBar } from "../composables/useStatusBar";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useWorkspacePluginMenu } from "../composables/useWorkspacePluginMenu";
import { useActiveOps } from "../composables/useActiveOps";
import { watch } from "vue";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const toast = useToast();
const statusBar = useStatusBar();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

// Sidebar honors per-profile EnabledTemplates curation - same filtered
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
  remove,
} = useTemplates();

const { draft, dirty, itemFieldOptions, save, reset } = useTemplateEditor();

// ArrowUp/ArrowDown step the template list, mirroring the list item's @pick.
useListKeyNav({
  keys: () => filenames.value,
  current: () => selectedFilename.value ?? "",
  select: (name) => { selectedFilename.value = name; },
  container: () => listScrollEl.value,
});

const { listScrollEl } = useTemplateSelection(filenames, selectedFilename);

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
    // stays stable - Vue propagates the new entry to the matching
    // TemplateListItem via its :template prop.
    await refreshOne(fn);
    // Notify other workspaces (notably StorageWorkspace) that this
    // template was saved. The backend already re-derived every
    // form row in the index via OnTemplateChanged, so all the
    // listener needs to do is re-fetch its summaries.
    window.dispatchEvent(
      new CustomEvent("formidable:template-saved", { detail: { filename: fn } }),
    );
    return;
  }
  if (result.reason === "validation") {
    // One toast per error - same shape the original Formidable used.
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
  // "no-draft" - guarded by the early return at top, but kept exhaustive.
  toast.error("workspace.templates.save_error", ["?"]);
}

function doReset() {
  reset();
}

// hasGuidField gates the Enable Collection switch - collection mode
// requires a record-level guid for the wiki/API resolver, so we don't
// let users flip the toggle on without one. Mirrors backend
// validation.collectionGuidError; without this gate, the user reaches
// "Save" only to be rejected after the fact.
//
// Asymmetric: when Collection is already ON we let the user toggle it
// OFF even without a guid (recovery path for templates that somehow
// got into the broken state - e.g. a guid field was removed manually).
const hasGuidField = computed(() => {
  const fields = draft.value?.fields ?? [];
  return fields.some((f: Field) => f.type === "guid");
});
const collectionToggleDisabled = computed(() => {
  return !hasGuidField.value && !draft.value?.enable_collection;
});

const {
  open: createOpen,
  input: createInput,
  error: createError,
  openCreate,
  submitCreate,
} = useTemplateCreate();

// ── Field tree (extracted into TemplateFieldsSection) ───────────────
// Topbar "+ New Field" calls into the section via its exposed
// openAddField; the section emits update(flat) when the user edits or
// deletes, and we write back to draft.fields here so dirty-detection
// in useTemplateEditor sees the change.
const fieldsSection = ref<InstanceType<typeof TemplateFieldsSection> | null>(null);

function openAddField() {
  if (!draft.value) return;
  fieldsSection.value?.openAddField();
}

function onFieldsUpdate(flat: Field[]) {
  if (!draft.value) return;
  draft.value.fields = flat;
}

// ── Generate-template dialog ─────────────────────────────────────────
const generateOpen = ref(false);

// ── Cleanup-storage dialog (Utilities → Cleanup Storage) ────────────
const cleanupOpen = ref(false);

// ── Reindex collection (Utilities → Reindex Search Index) ───────────
// Force-reindexes the selected template's storage items: the backend
// re-reads every record from disk and rebuilds its index rows (search
// body, values, tags, facets, title). Cheap escape hatch for when the
// full-text index is suspected of having drifted, or after a build
// that changed how records are indexed.
// SSOT: the op-tracker (backend) owns "is a reindex running"; the local latch
// only covers the click gap before optrack:changed lands.
const { isRunning } = useActiveOps();
const reindexing = ref(false);
function reindexBusy(fn: string): boolean {
  return reindexing.value || isRunning("index:rescan:" + fn);
}
async function reindexCollection() {
  const fn = selectedFilename.value;
  if (!fn || reindexBusy(fn)) return;
  reindexing.value = true;
  try {
    await IndexSvc.RescanTemplate(fn);
    toast.success("workspace.templates.reindex.success", [fn]);
  } catch (e) {
    toast.error("workspace.templates.reindex.error", [backendErrMessage(e)]);
  } finally {
    reindexing.value = false;
  }
}

// ── PDF frontmatter utilities (Utilities → Inject / Migrate PDF FM) ──
// Inject prepends the canonical picoloom scaffold to markdown_template
// (refuses if a `---` block is already there). Migrate parses an
// existing eisvogel-style frontmatter and rewrites it into picoloom
// shape, surfacing a preview modal so the user can see the mappings,
// preserved-as-legacy keys, and warnings before applying. Both
// operations only edit the in-memory draft; the user still has to
// click Save on the template to persist.
// Migrate keeps its preview-and-confirm modal (the diff IS the point).
// Inject moves to the form-based InjectPDFFrontmatterDialog wizard -
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
  // Refuse if frontmatter already exists - direct user to Migrate.
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

// ── Setup-info tabs (Template Code / Sidebar Expression / Facets) ───
const setupTab = ref<"code" | "expression" | "facets" | "statistics" | "formulas" | "relations">("code");
const setupTabItems = computed(() => {
  const items = [
    { id: "code", label: t("workspace.templates.setup.template_code") },
    { id: "expression", label: t("workspace.templates.setup.sidebar_expression") },
    { id: "facets", label: t("workspace.templates.setup.facets") },
    { id: "statistics", label: t("workspace.templates.setup.statistics") },
    { id: "formulas", label: t("workspace.templates.setup.formulas") },
  ];
  if (draft.value?.enable_collection) {
    items.unshift({ id: "relations", label: t("workspace.templates.setup.relations") });
  }
  return items;
});

// Default tab focus: Relations when it's available (collection enabled),
// otherwise Template Code. Re-evaluated on template switch and when the
// collection toggle flips. Manual tab clicks are unaffected: this only
// fires when the selected template or the toggle actually changes.
watch(
  () => [selectedFilename.value, draft.value?.enable_collection] as const,
  ([, on]) => {
    setupTab.value = on ? "relations" : "code";
  },
  { immediate: true },
);

// ── Relations tab (extracted into TemplateRelationsTab) ──────────────
// The tab owns its own load/persist/editor/delete; here we keep only the global
// Reconcile (Utilities menu) and a reload signal bumped after it so an open tab
// refreshes.
const relationsReloadKey = ref(0);
const reconciling = ref(false);
async function reconcileRelations() {
  if (reconciling.value) return;
  reconciling.value = true;
  try {
    const rep = await RelationSvc.Reconcile();
    const created = rep.created?.length ?? 0;
    const healed = rep.edges_healed ?? 0;
    const conflicts = rep.conflicts?.length ?? 0;
    toast.success("workspace.templates.relations.reconcile.done", [String(created), String(healed), String(conflicts)]);
    if (conflicts > 0) {
      toast.error("workspace.templates.relations.reconcile.conflicts", [String(conflicts)]);
    }
    relationsReloadKey.value++;
  } catch (e) {
    toast.error("workspace.templates.relations.reconcile.error", [backendErrMessage(e)]);
  } finally {
    reconciling.value = false;
  }
}


// ── Formulas tab (extracted into TemplateFormulasTab) ───────────────

// ── Statistics tab (extracted into TemplateStatisticsTab) ────────────

// ── Facets tab (extracted into TemplateFacetsTab, incl. weightings) ──

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
const { buildMenu: buildPluginsMenu } = useWorkspacePluginMenu(
  "templates",
  () => selectedFilename.value ?? "",
);
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
      {
        id: "reindexCollection",
        labelKey: "menu.utilities.reindex",
        disabled: !selectedFilename.value || reindexBusy(selectedFilename.value),
        onClick: reindexCollection,
      },
      {
        id: "reconcileRelations",
        labelKey: "menu.utilities.reconcile_relations",
        disabled: reconciling.value,
        onClick: reconcileRelations,
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
  ...(buildPluginsMenu() ? [buildPluginsMenu()!] : []),
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

      <div ref="listScrollEl" class="sidebar-scroll">
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
            <FieldSelector
              :model-value="draft.item_field || ''"
              @update:model-value="(v) => (draft && (draft.item_field = v))"
              :fields="itemFieldOptions"
              :empty-label="t('workspace.templates.item_field_none')"
            />
          </FormRow>
          <div class="setup-tabs-block">
            <Tabs v-model="setupTab" :items="setupTabItems">
              <template #code>
                <TemplateCodeTab
                  v-if="draft"
                  :name="draft.name ?? ''"
                  :filename="selectedFilename ?? ''"
                  :markdown-template="draft.markdown_template ?? ''"
                  @update:markdown-template="(v) => { if (draft) draft.markdown_template = v; }"
                  @generate="generateOpen = true"
                />
              </template>

              <template #expression>
                <TemplateExpressionTab
                  v-if="draft"
                  :sidebar-expression="draft.sidebar_expression ?? ''"
                  :fields="draft.fields ?? []"
                  :facets="draft.facets ?? []"
                  :formulas="draft.formulas ?? []"
                  @update:sidebar-expression="(v) => { if (draft) draft.sidebar_expression = v; }"
                />
              </template>

              <template #facets>
                <TemplateFacetsTab
                  v-if="draft"
                  :facets="draft.facets ?? []"
                  :scalings="draft.scalings ?? []"
                  @update:facets="(v) => { if (draft) draft.facets = v; }"
                  @update:scalings="(v) => { if (draft) draft.scalings = v; }"
                />
              </template>

              <template #statistics>
                <TemplateStatisticsTab
                  v-if="draft"
                  :template="selectedFilename ?? ''"
                  :statistics="draft.statistics ?? []"
                  :fields="draft.fields ?? []"
                  :facets="draft.facets ?? []"
                  :formulas="draft.formulas ?? []"
                  :scalings="draft.scalings ?? []"
                  @update:statistics="(v) => { if (draft) draft.statistics = v; }"
                />
              </template>

              <template #formulas>
                <TemplateFormulasTab
                  v-if="draft"
                  :template="selectedFilename ?? ''"
                  :formulas="draft.formulas ?? []"
                  :fields="draft.fields ?? []"
                  :facets="draft.facets ?? []"
                  :scalings="draft.scalings ?? []"
                  @update:formulas="(v) => { if (draft) draft.formulas = v; }"
                />
              </template>

              <template #relations>
                <TemplateRelationsTab
                  :template="selectedFilename ?? ''"
                  :reload-key="relationsReloadKey"
                />
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

        <TemplateFieldsSection
          ref="fieldsSection"
          :fields="draft.fields ?? []"
          :facets="draft.facets ?? []"
          :formulas="draft.formulas ?? []"
          @update="onFieldsUpdate"
        />
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

  <!-- Inject PDF frontmatter - form-based wizard (toggles + dropdowns). -->
  <InjectPDFFrontmatterDialog
    :open="pdfInjectOpen"
    :template-name="selectedTemplate?.name"
    @cancel="pdfInjectOpen = false"
    @apply="applyInjectedScaffold"
  />

  <!-- Migrate PDF frontmatter - preview the eisvogel→picoloom rewrite
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

</template>

