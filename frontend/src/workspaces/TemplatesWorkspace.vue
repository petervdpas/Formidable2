<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import SplitPane from "../components/SplitPane.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import TemplateFieldsSection from "../components/TemplateFieldsSection.vue";
import GenerateTemplateDialog from "../components/GenerateTemplateDialog.vue";
import CleanupStorageDialog from "../components/CleanupStorageDialog.vue";
import InjectPDFFrontmatterDialog from "../components/InjectPDFFrontmatterDialog.vue";
import TemplateListItem from "../components/TemplateListItem.vue";
import ExpressionBuilderModal from "../components/ExpressionBuilderModal.vue";
import FacetEditorModal from "../components/FacetEditorModal.vue";
import StatisticsBuilderModal from "../components/StatisticsBuilderModal.vue";
import CompositeBuilderModal from "../components/CompositeBuilderModal.vue";
import ScalingBuilderModal from "../components/ScalingBuilderModal.vue";
import FormulaEditorModal from "../components/FormulaEditorModal.vue";
import StatGridDialog from "../components/stat/StatGridDialog.vue";
import { type Grid, type CompositeGrid } from "../components/stat/grid";
import { Service as StatSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat";
import type { CompositeSpec } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat/models";
import FacetIcon from "../components/FacetIcon.vue";
import { useFacetMeta } from "../composables/useFacetMeta";
import { Facet, Formula, Scaling, Statistic } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
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
import { Service as IndexSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/index";
import { Service as RelationSvc, Relation, Cardinality } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation";
import type { CardinalityOption } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation/models";
import { Service as DataproviderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";
import type { TemplateSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider/models";
import RelationEditorModal from "../components/RelationEditorModal.vue";
import { useTemplateValidation } from "../composables/useTemplateValidation";
import type { FieldRef } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/builder";
import { backendErrMessage } from "../utils/backendError";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
  TextareaField,
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

const {
  errorDiagnostic: templateErrorDiagnostic,
  warningDiagnostics: templateWarningDiagnostics,
  isOK: templateValidationOK,
} = useTemplateValidation(() => draft.value?.markdown_template ?? "");

// ── Expression-builder dialog ────────────────────────────────────────
// Visual builder for sidebar_expression. The dialog is the only way
// to edit the source - the textarea is rendered read-only - so the
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
// strict builder Parse - i.e. it's a legacy shape (array-wrapped
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
    if (detail) toast.error(detail);
  }
}

watch(
  () => draft.value?.sidebar_expression,
  () => {
    void recheckExpressionParseable();
  },
  { immediate: true },
);

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

// ── Relations (sidecar, persisted immediately, NOT part of the draft) ──
const relations = ref<Relation[]>([]);
const collectionTemplates = ref<TemplateSummary[]>([]);
const relationEditorOpen = ref(false);
const editingRelationIndex = ref(-1);
const editingRelation = ref<Relation>(new Relation({ to: "", cardinality: Cardinality.OneToMany }));

// Cardinality options come from the backend (value + label key); the label is
// localized here. No frontend value->key mapping.
const cardinalityChoices = ref<CardinalityOption[]>([]);
void RelationSvc.Cardinalities().then((o) => (cardinalityChoices.value = o ?? []));
const cardinalityOptions = computed(() =>
  cardinalityChoices.value.map((o) => ({ value: o.value, label: t(o.label_key) })),
);
function cardinalityLabel(c: string): string {
  const opt = cardinalityChoices.value.find((o) => o.value === c);
  return opt ? t(opt.label_key) : c;
}
function relationTargetLabel(to: string): string {
  const s = collectionTemplates.value.find((x) => x.filename === to);
  return s?.name || s?.stem || to;
}

async function loadRelations(fn: string | null) {
  relations.value = [];
  if (!fn) return;
  try {
    collectionTemplates.value = await DataproviderSvc.ListCollectionTemplates();
    relations.value = (await RelationSvc.GetRelations(fn)) ?? [];
  } catch (e) {
    relations.value = [];
    toast.error(backendErrMessage(e));
  }
}
watch(() => selectedFilename.value, (fn) => void loadRelations(fn ?? null), { immediate: true });

// Editor target options: collection templates minus targets already linked
// (except the one being edited), so a duplicate relation can't be picked.
const relationTargetOptions = computed(() => {
  const used = new Set(
    relations.value
      .filter((_, i) => i !== editingRelationIndex.value)
      .map((r) => r.to),
  );
  return collectionTemplates.value
    .filter((s) => !used.has(s.filename))
    .map((s) => ({ value: s.filename, label: s.name || s.stem }));
});

function openAddRelation() {
  editingRelationIndex.value = -1;
  editingRelation.value = new Relation({ to: "", cardinality: Cardinality.OneToMany });
  relationEditorOpen.value = true;
}
function openEditRelation(idx: number) {
  const r = relations.value[idx];
  if (!r) return;
  editingRelationIndex.value = idx;
  editingRelation.value = new Relation({
    to: r.to,
    cardinality: r.cardinality,
    inverse: r.inverse,
  });
  relationEditorOpen.value = true;
}

// Immediate persist: write the whole relation set to the sidecar; revert the
// optimistic update if the backend rejects.
async function persistRelations(next: Relation[]) {
  const fn = selectedFilename.value;
  if (!fn) return;
  const prev = relations.value;
  relations.value = next;
  try {
    await RelationSvc.SetRelations(fn, next);
  } catch (e) {
    relations.value = prev;
    toast.error(backendErrMessage(e));
  }
}
function removeRelation(idx: number) {
  void persistRelations(relations.value.filter((_, i) => i !== idx));
}
function applyRelation(rel: Relation) {
  const next =
    editingRelationIndex.value < 0
      ? [...relations.value, rel]
      : relations.value.map((existing, i) =>
          i === editingRelationIndex.value ? rel : existing,
        );
  void persistRelations(next);
}

// ── Formula fields: named per-record computed fields (datacore-evaluated) ──
const formulaEditorOpen = ref(false);
const editingFormulaIndex = ref(-1);
const editingFormula = ref<Formula | null>(null);

function openAddFormula() {
  if (!draft.value) return;
  editingFormulaIndex.value = -1;
  editingFormula.value = null;
  formulaEditorOpen.value = true;
}

function openEditFormula(idx: number) {
  if (!draft.value) return;
  const f = draft.value.formulas?.[idx];
  if (!f) return;
  editingFormulaIndex.value = idx;
  editingFormula.value = new Formula({ key: f.key, label: f.label, type: f.type, expression: f.expression });
  formulaEditorOpen.value = true;
}

function removeFormula(idx: number) {
  if (!draft.value) return;
  const cur = draft.value.formulas ?? [];
  draft.value.formulas = [...cur.slice(0, idx), ...cur.slice(idx + 1)];
}

function applyFormula(f: Formula) {
  if (!draft.value) return;
  const cur = draft.value.formulas ?? [];
  if (editingFormulaIndex.value < 0) {
    draft.value.formulas = [...cur, f];
  } else {
    draft.value.formulas = cur.map((existing, i) => (i === editingFormulaIndex.value ? f : existing));
  }
  formulaEditorOpen.value = false;
}

// ── Statistical Engine: named statistical objects on the template ─────
// Edits one statistic at a time via StatisticsBuilderModal. The builder
// round-trips the DSL string through the backend (stat.Compile/Parse).
const statBuilderOpen = ref(false);
const editingStatIndex = ref(-1);
const editingStat = ref<Statistic | null>(null);

function openAddStatistic() {
  if (!draft.value) return;
  editingStatIndex.value = -1;
  editingStat.value = null;
  statBuilderOpen.value = true;
}

function openEditStatistic(idx: number) {
  if (!draft.value) return;
  const s = draft.value.statistics?.[idx];
  if (!s) return;
  editingStatIndex.value = idx;
  // Reopen each object kind in its own builder.
  if (s.composite) {
    editingComposite.value = s;
    compositeBuilderOpen.value = true;
    return;
  }
  editingStat.value = new Statistic({ name: s.name, label: s.label, dsl: s.dsl });
  statBuilderOpen.value = true;
}

function removeStatistic(idx: number) {
  if (!draft.value) return;
  const cur = draft.value.statistics ?? [];
  draft.value.statistics = [...cur.slice(0, idx), ...cur.slice(idx + 1)];
}

function applyStatistic(s: Statistic) {
  if (!draft.value) return;
  const cur = draft.value.statistics ?? [];
  if (editingStatIndex.value < 0) {
    draft.value.statistics = [...cur, s];
  } else {
    draft.value.statistics = cur.map((existing, i) => (i === editingStatIndex.value ? s : existing));
  }
  statBuilderOpen.value = false;
}

// ── Composite (hop route) objects: authored via CompositeBuilderModal,
// which is driven by the backend's CompositeOptions (only valid parent/child
// links are offered). Shares the statistics list + apply path with the DSL
// builder; editingStatIndex steers insert vs replace for both.
const compositeBuilderOpen = ref(false);
const editingComposite = ref<Statistic | null>(null);

function openAddComposite() {
  if (!draft.value) return;
  editingStatIndex.value = -1;
  editingComposite.value = null;
  compositeBuilderOpen.value = true;
}

function applyComposite(s: Statistic) {
  applyStatistic(s);
  compositeBuilderOpen.value = false;
}

// ── Scaling objects (reusable weightings): a facet weighting subsystem. Each
// facet owns a single weighting, edited inline from its row. A scaling has no
// grid of its own; the expression engine reads it as S["name"] and the
// Statistical Engine references it through the DSL scale "<name>" clause. Stored
// in draft.scalings (top-level), keyed to a facet by source.
const scalingBuilderOpen = ref(false);
const editingScalingIndex = ref(-1);
const editingScaling = ref<Scaling | null>(null);
const editingScalingFacet = ref<Facet | null>(null);
const scalings = computed(() => draft.value?.scalings ?? []);

// The weighting whose source is this facet (each facet owns at most one).
function scalingIndexForFacet(key: string): number {
  return (draft.value?.scalings ?? []).findIndex(
    (s) => s.source.kind === "facet" && s.source.key === key,
  );
}
function scalingForFacet(key: string): Scaling | null {
  const i = scalingIndexForFacet(key);
  return i >= 0 ? (draft.value?.scalings?.[i] ?? null) : null;
}

function openFacetWeighting(facet: Facet) {
  if (!draft.value) return;
  const idx = scalingIndexForFacet(facet.key);
  editingScalingIndex.value = idx;
  editingScaling.value = idx >= 0 ? (draft.value.scalings?.[idx] ?? null) : null;
  editingScalingFacet.value = facet;
  scalingBuilderOpen.value = true;
}

function removeScaling(idx: number) {
  if (!draft.value || idx < 0) return;
  const cur = draft.value.scalings ?? [];
  draft.value.scalings = [...cur.slice(0, idx), ...cur.slice(idx + 1)];
}

function applyScaling(s: Scaling) {
  if (!draft.value) return;
  const cur = draft.value.scalings ?? [];
  if (editingScalingIndex.value < 0) {
    draft.value.scalings = [...cur, s];
  } else {
    draft.value.scalings = cur.map((existing, i) => (i === editingScalingIndex.value ? s : existing));
  }
  scalingBuilderOpen.value = false;
}

function onRemoveScaling() {
  removeScaling(editingScalingIndex.value);
  scalingBuilderOpen.value = false;
}

// View an evaluated statistic. Uses EvaluateDSL on the draft's current
// DSL so it works on unsaved edits too (the statistic's own row need not
// be persisted; it only reads the template's already-indexed values).
const statViewOpen = ref(false);
const statViewGrid = ref<Grid | CompositeGrid | null>(null);
const statViewTitle = ref("");

async function openViewStatistic(s: Statistic) {
  const tpl = selectedFilename.value;
  if (!tpl) return;
  try {
    // A composite previews its spec inline (its parent + children are
    // already saved); a plain object evaluates its DSL.
    const grid = s.composite
      ? await StatSvc.EvaluateCompositeSpec(tpl, s.composite as unknown as CompositeSpec)
      : await StatSvc.EvaluateDSL(tpl, s.dsl);
    statViewGrid.value = grid as unknown as Grid | CompositeGrid;
    statViewTitle.value = s.label || s.name;
    statViewOpen.value = true;
  } catch (e) {
    toast.error("workspace.templates.statistics.view_failed", [backendErrMessage(e)]);
  }
}

// ── Facet editor dialog ──────────────────────────────────────────────
// Edits one facet at a time. editingIndex = -1 means "adding a new
// facet"; ≥0 means "editing the facet at that index in draft.facets".
// Limits come from the backend via useFacetMeta - no static mirrors.
const { maxFacets, icons: facetIcons } = useFacetMeta();
const defaultFacetIcon = computed(() => facetIcons.value[0] ?? "fa-flag");
const facetEditorOpen = ref(false);
const editingFacetIndex = ref(-1);
const editingFacet = ref<Facet>(new Facet({ key: "", icon: "fa-flag", options: [] }));

function openAddFacet() {
  if (!draft.value) return;
  if ((draft.value.facets?.length ?? 0) >= maxFacets.value) return;
  editingFacetIndex.value = -1;
  editingFacet.value = new Facet({
    key: "",
    icon: defaultFacetIcon.value,
    options: [],
  });
  facetEditorOpen.value = true;
}

function openEditFacet(idx: number) {
  if (!draft.value) return;
  const f = draft.value.facets?.[idx];
  if (!f) return;
  editingFacetIndex.value = idx;
  editingFacet.value = new Facet({
    key: f.key,
    icon: f.icon,
    options: (f.options ?? []).map((o) => ({ label: o.label, color: o.color })),
  });
  facetEditorOpen.value = true;
}

function removeFacet(idx: number) {
  if (!draft.value) return;
  const cur = draft.value.facets ?? [];
  draft.value.facets = [...cur.slice(0, idx), ...cur.slice(idx + 1)];
}

function applyFacet(f: Facet) {
  if (!draft.value) return;
  const cur = draft.value.facets ?? [];
  if (editingFacetIndex.value < 0) {
    draft.value.facets = [...cur, f];
  } else {
    draft.value.facets = cur.map((existing, i) => (i === editingFacetIndex.value ? f : existing));
  }
}

const otherFacetKeys = computed(() => {
  const cur = draft.value?.facets ?? [];
  return cur
    .filter((_, i) => i !== editingFacetIndex.value)
    .map((f) => f.key);
});

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
                <div class="setup-tab-pane">
                  <CodeEditor
                    v-model="draft.markdown_template"
                    lang="markdown"
                    :height="180"
                    :title="`${draft.name || selectedFilename} • ${t('workspace.templates.setup.template_code')}`"
                  />
                  <div
                    v-if="draft.markdown_template && draft.markdown_template.trim()"
                    class="template-validate-status"
                  >
                    <p
                      v-if="templateErrorDiagnostic"
                      class="template-validate-line error"
                    >
                      {{
                        templateErrorDiagnostic.line
                          ? t('workspace.templates.setup.validate.error_line', [String(templateErrorDiagnostic.line), templateErrorDiagnostic.message])
                          : t('workspace.templates.setup.validate.error_no_line', [templateErrorDiagnostic.message])
                      }}
                    </p>
                    <p
                      v-for="(w, i) in templateWarningDiagnostics"
                      :key="`warn-${i}-${w.helper}`"
                      class="template-validate-line warning"
                    >
                      {{ w.message }}
                    </p>
                    <p
                      v-if="!templateErrorDiagnostic && templateValidationOK"
                      class="template-validate-line ok"
                    >
                      {{ t('workspace.templates.setup.validate.ok') }}
                    </p>
                  </div>
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

              <template #facets>
                <div class="setup-tab-pane">
                  <p
                    v-if="!draft.facets || draft.facets.length === 0"
                    class="muted small"
                  >
                    {{ t('workspace.templates.facets.summary_empty') }}
                  </p>
                  <draggable
                    v-else
                    v-model="draft.facets"
                    tag="ul"
                    class="facet-rows"
                    handle=".dnd-handle"
                    :animation="150"
                    ghost-class="dnd-ghost"
                    chosen-class="dnd-chosen"
                    drag-class="dnd-drag"
                    item-key="key"
                  >
                    <template #item="{ element: f, index: i }">
                      <li class="facet-row">
                        <span class="dnd-handle" aria-hidden="true">☰</span>
                        <FacetIcon :icon="f.icon" class="facet-row-icon" />
                        <span class="facet-row-key mono">{{ f.key }}</span>
                        <span class="muted small facet-row-summary">
                          {{ t('workspace.templates.facets.options_count', [f.options.length]) }}
                        </span>
                        <code
                          v-if="scalingForFacet(f.key)"
                          class="facet-row-weighting mono"
                          :title="t('workspace.templates.scalings.intro')"
                        >S["{{ scalingForFacet(f.key)?.name }}"]</code>
                        <button
                          class="tool-btn"
                          type="button"
                          :title="t('workspace.templates.facets.edit')"
                          @click="openEditFacet(i)"
                        >{{ t('workspace.templates.facets.edit') }}</button>
                        <button
                          class="tool-btn"
                          type="button"
                          :class="{ 'is-active': !!scalingForFacet(f.key) }"
                          :title="t('workspace.templates.scalings.edit')"
                          @click="openFacetWeighting(f)"
                        >{{ t('workspace.templates.scalings.button') }}</button>
                        <button
                          class="tool-btn danger"
                          type="button"
                          :title="t('workspace.templates.facets.remove')"
                          @click="removeFacet(i)"
                        >×</button>
                      </li>
                    </template>
                  </draggable>
                  <div class="setup-tab-actions">
                    <span class="muted small">
                      {{ t('workspace.templates.facets.counter',
                           [draft.facets?.length ?? 0, maxFacets]) }}
                    </span>
                    <button
                      class="tool-btn"
                      type="button"
                      :disabled="(draft.facets?.length ?? 0) >= maxFacets"
                      @click="openAddFacet"
                    >+ {{ t('workspace.templates.facets.add') }}</button>
                  </div>
                  <p class="muted small facets-weighting-hint">
                    {{ t('workspace.templates.scalings.intro') }}
                  </p>
                </div>
              </template>

              <template #statistics>
                <div class="setup-tab-pane">
                  <p
                    v-if="!draft.statistics || draft.statistics.length === 0"
                    class="muted small"
                  >
                    {{ t('workspace.templates.statistics.empty') }}
                  </p>
                  <draggable
                    v-else
                    v-model="draft.statistics"
                    tag="ul"
                    class="stat-rows"
                    handle=".dnd-handle"
                    :animation="150"
                    ghost-class="dnd-ghost"
                    chosen-class="dnd-chosen"
                    drag-class="dnd-drag"
                    item-key="name"
                  >
                    <template #item="{ element: s, index: i }">
                      <li class="stat-row">
                        <span class="dnd-handle" aria-hidden="true">☰</span>
                        <span class="stat-row-name">{{ s.label || s.name }}</span>
                        <code v-if="s.composite" class="stat-row-dsl">{{ t('workspace.templates.statistics.composite_summary', [s.composite.parent, s.composite.edges.length]) }}</code>
                        <code v-else class="stat-row-dsl">{{ s.dsl }}</code>
                        <button
                          class="tool-btn"
                          type="button"
                          :title="t('workspace.templates.statistics.view')"
                          @click="openViewStatistic(s)"
                        >{{ t('workspace.templates.statistics.view') }}</button>
                        <button
                          class="tool-btn"
                          type="button"
                          :title="t('workspace.templates.statistics.edit')"
                          @click="openEditStatistic(i)"
                        >{{ t('workspace.templates.statistics.edit') }}</button>
                        <button
                          class="tool-btn danger"
                          type="button"
                          :title="t('workspace.templates.statistics.remove')"
                          @click="removeStatistic(i)"
                        >×</button>
                      </li>
                    </template>
                  </draggable>
                  <div class="setup-tab-actions">
                    <button class="tool-btn" type="button" @click="openAddStatistic">
                      + {{ t('workspace.templates.statistics.add') }}
                    </button>
                    <button class="tool-btn" type="button" @click="openAddComposite">
                      + {{ t('workspace.templates.statistics.add_composite') }}
                    </button>
                  </div>
                </div>
              </template>

              <template #formulas>
                <div class="setup-tab-pane">
                  <p
                    v-if="!draft.formulas || draft.formulas.length === 0"
                    class="muted small"
                  >
                    {{ t('workspace.templates.formulas.empty') }}
                  </p>
                  <draggable
                    v-else
                    v-model="draft.formulas"
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
                      <li class="formula-row">
                        <span class="dnd-handle" aria-hidden="true">☰</span>
                        <span class="formula-row-name">{{ f.label || f.key }}</span>
                        <span class="formula-row-type">{{ f.type }}</span>
                        <code class="formula-row-expr">{{ f.expression }}</code>
                        <button
                          class="tool-btn"
                          type="button"
                          :title="t('workspace.templates.formulas.edit')"
                          @click="openEditFormula(i)"
                        >{{ t('workspace.templates.formulas.edit') }}</button>
                        <button
                          class="tool-btn danger"
                          type="button"
                          :title="t('workspace.templates.formulas.remove')"
                          @click="removeFormula(i)"
                        >×</button>
                      </li>
                    </template>
                  </draggable>
                  <div class="setup-tab-actions">
                    <button class="tool-btn" type="button" @click="openAddFormula">
                      + {{ t('workspace.templates.formulas.add') }}
                    </button>
                  </div>
                </div>
              </template>

              <template #relations>
                <div class="setup-tab-pane">
                  <p class="muted small setup-tab-help">
                    {{ t('workspace.templates.relations.help') }}
                  </p>
                  <p
                    v-if="relations.length === 0"
                    class="muted small"
                  >
                    {{ t('workspace.templates.relations.empty') }}
                  </p>
                  <ul v-else class="relation-rows">
                    <li
                      v-for="(rel, i) in relations"
                      :key="rel.to"
                      class="relation-row"
                    >
                      <span class="relation-row-target">{{ relationTargetLabel(rel.to) }}</span>
                      <code class="relation-row-cardinality mono">{{ cardinalityLabel(rel.cardinality) }}</code>
                      <button
                        class="tool-btn"
                        type="button"
                        :title="t('workspace.templates.relations.edit')"
                        @click="openEditRelation(i)"
                      >{{ t('workspace.templates.relations.edit') }}</button>
                      <button
                        class="tool-btn danger"
                        type="button"
                        :title="t('workspace.templates.relations.remove')"
                        @click="removeRelation(i)"
                      >×</button>
                    </li>
                  </ul>
                  <div class="setup-tab-actions">
                    <button class="tool-btn" type="button" @click="openAddRelation">
                      + {{ t('workspace.templates.relations.add') }}
                    </button>
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

  <!-- Expression builder dialog: visual builder for sidebar_expression -->
  <ExpressionBuilderModal
    v-if="draft"
    :open="expressionBuilderOpen"
    :fields="draft.fields ?? []"
    :facets="draft.facets ?? []"
    :formulas="draft.formulas ?? []"
    :initial="draft.sidebar_expression"
    @close="expressionBuilderOpen = false"
    @apply="applyExpressionBuilder"
    @clear="clearExpressionSource"
  />

  <!-- Facet editor: edits one facet (key + icon + options) at a time -->
  <FacetEditorModal
    v-if="draft"
    :open="facetEditorOpen"
    :initial="editingFacet"
    :existing-keys="otherFacetKeys"
    @close="facetEditorOpen = false"
    @apply="applyFacet"
  />

  <!-- Statistical Engine: edits one named statistical object at a time -->
  <StatisticsBuilderModal
    v-if="draft"
    :open="statBuilderOpen"
    :template="selectedFilename || ''"
    :fields="draft.fields ?? []"
    :facets="draft.facets ?? []"
    :formulas="draft.formulas ?? []"
    :scalings="scalings"
    :initial="editingStat"
    @close="statBuilderOpen = false"
    @apply="applyStatistic"
  />

  <!-- Composite (hop route) builder: parent + per-branch children, driven
       by the backend's CompositeOptions -->
  <CompositeBuilderModal
    v-if="draft"
    :open="compositeBuilderOpen"
    :template="selectedFilename || ''"
    :statistics="draft.statistics ?? []"
    :initial="editingComposite"
    @close="compositeBuilderOpen = false"
    @apply="applyComposite"
  />

  <!-- Weighting builder: a facet-locked per-option factor map -->
  <ScalingBuilderModal
    v-if="draft"
    :open="scalingBuilderOpen"
    :facet="editingScalingFacet"
    :initial="editingScaling"
    @close="scalingBuilderOpen = false"
    @apply="applyScaling"
    @remove="onRemoveScaling"
  />

  <!-- Formula field editor: key + type + expression with a live preview -->
  <FormulaEditorModal
    v-if="draft"
    :open="formulaEditorOpen"
    :template="selectedFilename || ''"
    :fields="draft.fields ?? []"
    :facets="draft.facets ?? []"
    :scalings="draft.scalings ?? []"
    :initial="editingFormula"
    @close="formulaEditorOpen = false"
    @apply="applyFormula"
  />

  <RelationEditorModal
    :open="relationEditorOpen"
    :initial="editingRelation"
    :is-edit="editingRelationIndex >= 0"
    :targets="relationTargetOptions"
    :cardinalities="cardinalityOptions"
    @close="relationEditorOpen = false"
    @apply="applyRelation"
  />

  <!-- Evaluated-statistic viewer (rank-N grid + composite sunburst) -->
  <StatGridDialog
    :open="statViewOpen"
    :title="statViewTitle"
    :grid="statViewGrid"
    :facets="draft?.facets ?? []"
    @close="statViewOpen = false"
  />
</template>

<style scoped>
.generate-template-row {
  margin-top: 0.5rem;
  display: flex;
  justify-content: flex-start;
}
</style>

