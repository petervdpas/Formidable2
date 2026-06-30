<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import SplitPane from "../components/SplitPane.vue";
import Badge from "../components/Badge.vue";
import CopyButton from "../components/CopyButton.vue";
import EntryNameDialog from "../components/EntryNameDialog.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import UnsavedChangesDialog from "../components/UnsavedChangesDialog.vue";
import RightSlideout from "../components/RightSlideout.vue";
import RenderedHtml from "../components/RenderedHtml.vue";
import ImportDialog from "../components/ImportDialog.vue";
import ExportDialog from "../components/ExportDialog.vue";
import ExportPDFDialog from "../components/ExportPDFDialog.vue";
import QueryDialog from "../components/QueryDialog.vue";
import DatacoreGraphDialog from "../components/DatacoreGraphDialog.vue";
import { SelectField } from "../components/fields";
import FilteredCount from "../components/FilteredCount.vue";
import StorageListItem from "../components/StorageListItem.vue";
import VirtualList from "../components/VirtualList.vue";
import draggable from "vuedraggable";
import StorageSearch from "../components/StorageSearch.vue";
import StorageTagFilter from "../components/StorageTagFilter.vue";
import StorageFacetFilter from "../components/StorageFacetFilter.vue";
import StorageMetaBlock from "../components/StorageMetaBlock.vue";
import StorageDataForm from "../components/StorageDataForm.vue";
import Popup from "../components/Popup.vue";
import RelationLinksPanel from "../components/RelationLinksPanel.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useTemplates } from "../composables/useTemplates";
import { useFormView } from "../composables/useFormView";
import { useConfig } from "../composables/useConfig";
import { useToast } from "../composables/useToast";
import { useStatusBar } from "../composables/useStatusBar";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useWorkspacePluginMenu } from "../composables/useWorkspacePluginMenu";
import { useFormidableLink } from "../composables/useFormidableLink";
import { FACET_CONTEXT_KEY } from "../composables/facetContext";
import { FORM_VALUES_KEY } from "../composables/formValues";
import { FORM_FIELD_OPS_KEY } from "../composables/formFieldOps";
import { useActiveOps } from "../composables/useActiveOps";
import { useListKeyNav } from "../composables/useListKeyNav";
import { setNavGuard } from "../composables/useNavGuard";
import { usePDFActivation } from "../composables/usePDFActivation";
import { Service as ExpressionSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
import { Service as FormSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { Service as SystemSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as IndexSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/index";
import { FormulaService as FormulaSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/app";
import { backendErrMessage } from "../utils/backendError";
import type { FormSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { FacetState } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { CollectionItem } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";
import type { Result as ExpressionResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config, update: updateConfig } = useConfig();
// Storage picker shows only the templates the active profile has
// enabled in Settings → Templates - the filtered list comes pre-filtered
// from the backend (ConfigSvc.ListEnabledTemplates), so the picker
// always reflects the current profile's curation with no JS-side
// intersection.
const { enabledFilenames: templateFilenames, cache: templateCache } = useTemplates();
const { view, draft, dirty, canUndo, canRedo, undo, redo, open, close, save, reset, remove } = useFormView();
const toast = useToast();
const statusBar = useStatusBar();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

// ── Unsaved-changes guard ────────────────────────────────────────────
// One dialog drives every "leaving a dirty form" path: switching entry
// or template (the watcher below), switching workspace (App.vue via
// useNavGuard), and closing the app (backend WindowClosing hook ->
// "app:close-requested" -> App.vue). guardLeave resolves true when it's
// safe to proceed and false to abort (Cancel).
type LeaveChoice = "save" | "discard" | "cancel";
const leavePromptOpen = ref(false);
let leaveResolver: ((c: LeaveChoice) => void) | null = null;

function askLeave(): Promise<LeaveChoice> {
  leavePromptOpen.value = true;
  return new Promise<LeaveChoice>((resolve) => {
    leaveResolver = resolve;
  });
}

function resolveLeave(choice: LeaveChoice) {
  leavePromptOpen.value = false;
  leaveResolver?.(choice);
  leaveResolver = null;
}

async function guardLeave(): Promise<boolean> {
  if (!dirty.value) return true;
  const choice = await askLeave();
  if (choice === "cancel") return false;
  if (choice === "save") {
    const result = await save();
    if (!result.ok) {
      toast.error("workspace.storage.save.error", [result.message ?? ""]);
      return false; // keep the user on the form rather than lose data
    }
    return true;
  }
  reset(); // discard: drop the draft edits so dirty clears
  return true;
}

// Register the guard while this workspace is mounted; App.vue's
// rail-switch and app-close handlers consult it via confirmLeave().
setNavGuard(guardLeave);
onBeforeUnmount(() => {
  setNavGuard(null);
  void SystemSvc.SetUnsavedChanges(false);
});

// Mirror the active form's dirty state into the backend so the
// WindowClosing hook knows whether to veto an OS-driven close.
watch(dirty, (d) => { void SystemSvc.SetUnsavedChanges(d); }, { immediate: true });

// Active template's filename - provided downward so per-type field
// components that need it (image saves into <storage>/<tplName>/images/,
// for example) can inject without prop-drilling through the renderer.
const currentTemplateFilename = computed(
  () => draft.value?.template?.filename ?? "",
);
provide("templateFilename", currentTemplateFilename);

// ── Active template selection ────────────────────────────────────────
// Read-only computed off config - onTemplateChange below writes back
// when the dropdown fires. Switching templates also clears the
// selected datafile so we don't try to open a form whose schema no
// longer matches.
const selectedTemplate = computed<string>(
  () => config.value?.selected_template ?? "",
);

// Per-record relation linking (Relations popover by the STORAGE label). Only
// available for a collection template with a record open (edges link records).
const currentRecordId = computed<string>(() => draft.value?.meta?.id ?? "");
const canLinkRelations = computed<boolean>(
  () => !!currentRecordId.value && !!draft.value?.template?.enable_collection,
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

const hasTagsField = computed(() => {
  const tpl = templateCache.value.get(selectedTemplate.value);
  return !!tpl?.fields?.some((f) => f.type === "tags");
});

const facets = computed(() => {
  const tpl = templateCache.value.get(selectedTemplate.value);
  return tpl?.facets ?? [];
});

// Per-facet state update on the active draft. Mutates the FacetState
// entry under draft.meta.facets[key], creating the map on first write.
function onFacetStateChange(key: string, state: FacetState) {
  if (!draft.value?.meta) return;
  if (!draft.value.meta.facets) draft.value.meta.facets = {};
  draft.value.meta.facets[key] = new FacetState({
    set: state.set,
    selected: state.selected ?? "",
  });
}

// Bridge for inline virtual facet field renderers (FormFieldFacet).
// Same data, same writer as the StorageMetaBlock corner picker, so
// both setters stay in sync without a second source of truth.
const facetsStateView = computed<{ [key: string]: FacetState | undefined }>(() => {
  return draft.value?.meta?.facets ?? {};
});

provide(FACET_CONTEXT_KEY, {
  facets,
  state: facetsStateView,
  onChange: onFacetStateChange,
});

// Bridge for inline virtual fields that project a sibling's value
// (FormFieldFormula reads its target from here). The live Compute action
// reads the SAVED record on the backend and writes the result into the
// target field, so it is blocked while the form is dirty or unsaved.
const formValuesView = computed<Record<string, unknown>>(() => draft.value?.values ?? {});
const formSaved = computed<boolean>(() => view.value?.saved === true);

// target field key -> live formula field key. The Compute button renders under
// the target field; the formula field itself stays invisible in the form. Every
// field is required to have a key, so the formula field's key is a safe handle.
const liveFormulaTargets = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {};
  for (const f of draft.value?.template?.fields ?? []) {
    if (f.type === "formula" && f.trigger === "live" && f.target_key && f.key) {
      out[f.target_key] = f.key;
    }
  }
  return out;
});

async function computeFormulaField(fieldKey: string): Promise<void> {
  const tpl = draft.value?.template?.filename;
  const df = draft.value?.datafile;
  if (!tpl || !df || !draft.value || !draft.value.values) return;
  if (dirty.value || !formSaved.value) {
    toast.error("formula.field.compute_dirty");
    return;
  }
  try {
    const res = await FormulaSvc.ComputeField(tpl, df, fieldKey);
    draft.value.values[res.target_key] = res.value;
  } catch (e) {
    toast.error(backendErrMessage(e));
  }
}

provide(FORM_VALUES_KEY, {
  values: formValuesView,
  dirty,
  saved: formSaved,
  liveFormulaTargets,
  compute: computeFormulaField,
});

// List/table sort + dedup. The widget hands us only its field key; we
// send the pointer (template, datafile, field) to the backend, which
// fetches that field from the saved record, sorts/dedups it and returns
// the new value. We hand the value back to the widget, which applies it
// via update:modelValue; the normal Save persists it. The sort/dedup
// reads disk but never writes. See composables/formFieldOps.ts.
async function runFieldOp(
  call: (tpl: string, df: string) => Promise<unknown>,
): Promise<unknown | undefined> {
  const tpl = selectedTemplate.value;
  const df = draft.value?.datafile;
  if (!tpl || !df || draft.value?.saved !== true) {
    toast.error("workspace.storage.fieldop.unsaved");
    return undefined;
  }
  try {
    return await call(tpl, df);
  } catch (e) {
    toast.error("workspace.storage.fieldop.error", [backendErrMessage(e)]);
    return undefined;
  }
}

provide(FORM_FIELD_OPS_KEY, {
  sortField: (fieldKey, opts) =>
    runFieldOp((tpl, df) =>
      FormSvc.SortFieldValue(tpl, df, fieldKey, opts?.column ?? "", opts?.direction ?? "asc"),
    ),
  dedupField: (fieldKey, opts) =>
    runFieldOp((tpl, df) => FormSvc.DedupFieldValue(tpl, df, fieldKey, opts?.column ?? "")),
});

// ── Form list (sidebar) ──────────────────────────────────────────────
const summaries = ref<FormSummary[]>([]);
const listError = ref("");

// Sidebar sub-label items keyed by datafile. The workspace owns this
// map and hands each row's entry down as a prop - collapses what was
// N parallel EvaluateListOne calls into one EvaluateListMany on
// list load / Refresh. Single-row saves still take the cheap path
// (one EvaluateListOne) and update just that key. Reassigning the
// ref (rather than mutating a Map in place) keeps reactivity simple
// for downstream template lookups.
const sidebarItems = ref<Map<string, ExpressionResult>>(new Map());

async function refreshSidebarItems(): Promise<void> {
  if (!config.value?.use_expressions || !selectedTemplate.value) {
    sidebarItems.value = new Map();
    return;
  }
  const filenames = summaries.value.map((s) => s.filename);
  if (filenames.length === 0) {
    sidebarItems.value = new Map();
    return;
  }
  try {
    const items = await ExpressionSvc.EvaluateListMany(
      selectedTemplate.value,
      filenames,
    );
    const next = new Map<string, ExpressionResult>();
    for (const it of items) {
      if (it?.filename) next.set(it.filename, it);
    }
    sidebarItems.value = next;
  } catch {
    sidebarItems.value = new Map();
  }
}

async function refreshSidebarItem(filename: string): Promise<void> {
  if (!config.value?.use_expressions || !selectedTemplate.value || !filename) return;
  try {
    const it = await ExpressionSvc.EvaluateListOne(selectedTemplate.value, filename);
    const next = new Map(sidebarItems.value);
    if (it?.filename) {
      next.set(it.filename, it);
    } else {
      next.delete(filename);
    }
    sidebarItems.value = next;
  } catch {
    const next = new Map(sidebarItems.value);
    next.delete(filename);
    sidebarItems.value = next;
  }
}

function dropSidebarItem(filename: string): void {
  if (!sidebarItems.value.has(filename)) return;
  const next = new Map(sidebarItems.value);
  next.delete(filename);
  sidebarItems.value = next;
}

async function refreshList() {
  if (!selectedTemplate.value) {
    summaries.value = [];
    sidebarItems.value = new Map();
    return;
  }
  listError.value = "";
  try {
    await FormSvc.EnsureFormDir(selectedTemplate.value);
    summaries.value = await FormSvc.ListForms(selectedTemplate.value);
    if (presentationMode.value) {
      await applySequenceOrder();
    }
    // Drop a stale `selected_data_file` if it doesn't exist in the
    // current template's storage. Without this, switching templates
    // (or coming back later after the form was deleted on disk by
    // sync/external means) leaves the workspace pointing at a
    // phantom file: the form view then renders default values under
    // the orphan filename, which looks broken. The config field is
    // global rather than per-template, so this guard is the only
    // place it can be reconciled.
    const df = selectedDataFile.value;
    if (df && !summaries.value.some((s) => s.filename === df)) {
      selectedDataFile.value = "";
    }
    await refreshSidebarItems();
    await scrollActiveIntoView();
  } catch (err) {
    listError.value = String(err);
    summaries.value = [];
    sidebarItems.value = new Map();
  }
}

// applySequenceOrder reorders the fetched summaries to match the backend's
// sequence order (the index can't ORDER BY a data field, so the form service
// does it in Go). Best-effort: on error the backend list order stands.
async function applySequenceOrder() {
  if (!selectedTemplate.value) return;
  try {
    const order = await FormSvc.SequenceOrder(selectedTemplate.value);
    const pos = new Map(order.map((f, i) => [f, i] as const));
    summaries.value = [...summaries.value].sort(
      (a, b) =>
        (pos.get(a.filename) ?? Number.MAX_SAFE_INTEGER) -
        (pos.get(b.filename) ?? Number.MAX_SAFE_INTEGER),
    );
  } catch {
    // keep the backend's order
  }
}

// onReorderChange handles a drag-drop within the presentation list. The moved
// record gets a fresh sequence value (the backend writes only that record, or
// renumbers when no gap is left); the optimistic local reorder keeps the row in
// place until the refresh reconciles against what the backend actually wrote.
async function onReorderChange(evt: {
  moved?: { element: FormSummary; oldIndex: number; newIndex: number };
}) {
  const moved = evt.moved;
  if (!moved || !selectedTemplate.value) return;
  const files = visibleSummaries.value.map((s) => s.filename);
  const [m] = files.splice(moved.oldIndex, 1);
  files.splice(moved.newIndex, 0, m);
  const byFile = new Map(summaries.value.map((s) => [s.filename, s] as const));
  summaries.value = files
    .map((f) => byFile.get(f))
    .filter((s): s is FormSummary => !!s);
  try {
    await FormSvc.ReorderSequence(
      selectedTemplate.value,
      moved.element.filename,
      files,
    );
  } catch (e) {
    toast.error(backendErrMessage(e));
  }
  await refreshList();
}

// normalizeSequence re-spreads the deck to clean 10/20/30 spacing (the cleanup
// for when many minimal-write moves have shrunk the gaps).
async function normalizeSequence() {
  if (!selectedTemplate.value) return;
  try {
    await FormSvc.NormalizeSequence(selectedTemplate.value);
    await refreshList();
  } catch (e) {
    toast.error(backendErrMessage(e));
  }
}

// User-triggered refresh - same backend path as the watch-driven
// refreshList, but surfaces success/failure as a toast (refreshList
// itself is silent because it runs on every template change).
async function doRefresh() {
  try {
    await refreshList();
    if (listError.value) {
      toast.error("toast.refresh.error", [listError.value]);
    } else {
      toast.success("toast.refresh.success");
    }
  } catch (err) {
    toast.error("toast.refresh.error", [String(err)]);
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

// After a pull/clone/reclone the form files on disk may have
// changed even when selectedTemplate didn't - re-read the list so
// the sidebar reflects upstream deletions/additions.
function onContextReloaded() {
  void refreshList();
}
// Backend-driven: a bulk write (cleanup Migrate/Repair) emits storage:changed
// with the affected template. Re-read the list so the sidebar and open form
// reflect the corrected meta on disk instead of a stale in-memory view.
let unsubStorageChanged: (() => void) | undefined;
function onStorageChanged(ev: unknown) {
  const data = (ev as { data?: unknown })?.data;
  const tpl = Array.isArray(data) ? data[0] : data;
  if (typeof tpl !== "string" || tpl === selectedTemplate.value) {
    void refreshList();
  }
}
// Templates workspace just saved a template. The backend's
// OnTemplateChanged already re-derived every form row in the index,
// so a plain re-fetch picks up the new title / expression sub-label
// / tags / facet projections without further coordination. Only act
// when the saved template is the one our workspace is showing.
function onTemplateSaved(e: Event) {
  const detail = (e as CustomEvent).detail as { filename?: string } | null;
  if (detail?.filename && detail.filename === selectedTemplate.value) {
    void refreshList();
  }
}
onMounted(() => {
  window.addEventListener("formidable:context-reloaded", onContextReloaded);
  window.addEventListener("formidable:template-saved", onTemplateSaved);
  unsubStorageChanged = Events.On("storage:changed", onStorageChanged);
});
onBeforeUnmount(() => {
  window.removeEventListener("formidable:context-reloaded", onContextReloaded);
  window.removeEventListener("formidable:template-saved", onTemplateSaved);
  unsubStorageChanged?.();
});

// Live-toggle: flipping use_expressions in Settings re-fetches (or
// clears) the sidebar items map without touching the row list.
watch(() => config.value?.use_expressions, async () => {
  await refreshSidebarItems();
});

// ── Selected datafile (persisted in config) ──────────────────────────
const selectedDataFile = computed<string>({
  get: () => config.value?.selected_data_file ?? "",
  set: (v) => { void updateConfig({ selected_data_file: v }); },
});

// `loaded*` track the (template, datafile) the current draft reflects,
// so the watcher can tell a real navigation from a no-op and revert the
// config selection when the user cancels. `reverting` suppresses the
// guard while we restore the prior selection.
let loadedTpl = "";
let loadedDf = "";
let reverting = false;

watch(
  [selectedTemplate, selectedDataFile],
  async ([tpl, df], oldVals) => {
    if (reverting) {
      reverting = false;
      return;
    }

    // Leaving a loaded form for a different selection: prompt if dirty.
    // On cancel, restore the previous config selection and bail.
    const changed = tpl !== loadedTpl || df !== loadedDf;
    if (changed && (loadedTpl || loadedDf) && draft.value) {
      const ok = await guardLeave();
      if (!ok) {
        reverting = true;
        void updateConfig({
          selected_template: loadedTpl,
          selected_data_file: loadedDf,
        });
        return;
      }
    }

    if (!tpl || !df) {
      close();
      loadedTpl = tpl;
      loadedDf = df;
      return;
    }
    // If the template changed, drop the prior form (different schema)
    // before loading the new one so we never render stale fields.
    const prevTpl = oldVals?.[0];
    if (prevTpl && prevTpl !== tpl) close();
    await open(tpl, df);
    loadedTpl = tpl;
    loadedDf = df;
    // `df` is the config-persisted stem; `draft.datafile` is the
    // actual on-disk filename (e.g. "projectstatus.meta.json").
    statusBar.setSelected(draft.value?.datafile ?? df);
  },
  { immediate: true },
);

// Scroll the active row into view exactly once per template context.
// VirtualList owns the settling retry (it queues the request until the
// viewport has a real height, which on first startup is only after the
// splash dismisses) and the index-based centering, so this just gates one
// scroll per template so save / refresh / clicks don't yank the viewport.
const virtualList = ref<{ scrollToKey: (k: string, o?: { center?: boolean }) => boolean } | null>(null);
let scrolledForTemplate: string | null = null;

watch(selectedTemplate, () => {
  scrolledForTemplate = null;
});

async function scrollActiveIntoView() {
  const tpl = selectedTemplate.value;
  const df = selectedDataFile.value;
  if (!tpl || !df) return;
  if (scrolledForTemplate === tpl) return;

  await nextTick();
  // scrollToKey returns false (and queues internally) when the viewport
  // isn't sized yet; mark done only once it actually centered the row.
  if (virtualList.value?.scrollToKey(df, { center: true })) {
    scrolledForTemplate = tpl;
  }
}

const { follow: followFormidable } = useFormidableLink();

async function pickForm(filename: string) {
  const tpl = selectedTemplate.value;
  if (!tpl || !filename) return;
  await followFormidable(`formidable://${tpl}:${filename}`);
}

// ArrowUp/ArrowDown step the visible record list. pickForm routes through the
// formidable:// link, so the unsaved-changes guard fires just as it does on a
// click.
useListKeyNav({
  keys: () => visibleSummaries.value.map((s) => s.filename),
  current: () => selectedDataFile.value,
  select: (filename) => { void pickForm(filename); },
  enabled: () => !!selectedTemplate.value,
  scrollTo: (key) => { virtualList.value?.scrollToKey(key); },
});

// ── Sidebar filters ─────────────────────────────────────────────────
// facetFilters: per-facet selected-label (or "" = no filter for that
// facet). A form passes when every active filter entry matches its
// meta.facets[key].selected with set=true.
const facetFilters = ref<Record<string, string>>({});
const tagFilter = ref("");

// ── Full-text search (opt-in via config.enable_full_text_search) ─────
// When enabled, the sidebar grows a search box that queries the FTS
// index for this collection. searchResults holds the backend's ranked
// matches (null = not searching, so the plain list shows). Search runs
// over the whole collection on the backend; the facet/tag filters above
// still compose on top of whichever set is showing.
const ftsEnabled = computed(() => !!config.value?.enable_full_text_search);
const searchQuery = ref("");
const searchResults = ref<FormSummary[] | null>(null);
let searchTimer: ReturnType<typeof setTimeout> | null = null;

async function runSearch(): Promise<void> {
  const tpl = selectedTemplate.value;
  const q = searchQuery.value.trim();
  if (!ftsEnabled.value || !tpl || q === "") {
    searchResults.value = null;
    return;
  }
  try {
    searchResults.value = await StorageSvc.SearchForms(tpl, q);
  } catch (err) {
    listError.value = backendErrMessage(err);
    searchResults.value = [];
  }
}

// Debounce keystrokes so we don't fire an FTS query per character.
watch(searchQuery, () => {
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => { void runSearch(); }, 180);
});
onBeforeUnmount(() => { if (searchTimer) clearTimeout(searchTimer); });

function clearSearch() {
  searchQuery.value = "";
  searchResults.value = null;
}

// Reset facet filters AND the search box when the active template
// changes - the new template's facets may differ (otherwise the
// sidebar looks mysteriously empty), and a stale query from the prior
// collection shouldn't carry over.
watch(selectedTemplate, () => {
  facetFilters.value = {};
  clearSearch();
});

// Show a filter chip per facet key only when ≥1 record actually has
// set=true for that key. Mirrors today's behavior for legacy flags.
const usedFacets = computed(() => {
  const used = new Set<string>();
  for (const s of summaries.value) {
    const m = s.meta?.facets;
    if (!m) continue;
    for (const [k, v] of Object.entries(m)) {
      if (v?.set) used.add(k);
    }
  }
  return facets.value.filter((f) => used.has(f.key));
});

function setFacetFilter(key: string, label: string) {
  if (label === "") {
    const next = { ...facetFilters.value };
    delete next[key];
    facetFilters.value = next;
  } else {
    facetFilters.value = { ...facetFilters.value, [key]: label };
  }
}

const hasActiveFilters = computed(
  () =>
    Object.values(facetFilters.value).some((v) => v !== "") ||
    tagFilter.value.trim() !== "",
);

function clearFilters() {
  facetFilters.value = {};
  tagFilter.value = "";
}

// Search (when active) narrows the base set the facet/tag filters then
// refine; otherwise the full list is the base. Search results keep the
// backend's relevance order.
const baseSummaries = computed(() =>
  ftsEnabled.value && searchResults.value !== null
    ? searchResults.value
    : summaries.value,
);

const visibleSummaries = computed(() => {
  let out = baseSummaries.value;
  const active = Object.entries(facetFilters.value).filter(([, v]) => v !== "");
  if (active.length > 0) {
    out = out.filter((s) => {
      const fs = s.meta?.facets;
      if (!fs) return false;
      return active.every(([key, label]) => {
        const entry = fs[key];
        return !!entry && entry.set && entry.selected === label;
      });
    });
  }
  const tokens = tagFilter.value
    .toLowerCase()
    .split(/[,\s]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  if (tokens.length > 0) {
    out = out.filter((s) => {
      const tags = (s.meta?.tags ?? []).map((t) => t.toLowerCase());
      return tokens.every((tok) => tags.some((tag) => tag.includes(tok)));
    });
  }
  return out;
});

// The sidebar's filtered records, as CollectionItems, for a self-relation's
// add picker in the Relations popover (RelationLinksPanel scopes self to this).
const sidebarRelationItems = computed<CollectionItem[]>(() =>
  visibleSummaries.value.map(
    (s) =>
      new CollectionItem({
        template: selectedTemplate.value,
        id: s.meta?.id ?? "",
        filename: s.filename,
        title: s.title,
      }),
  ),
);

// ── New Entry / Copy dialogs ─────────────────────────────────────────
// Both reuse EntryNameDialog (filename input + append-date + validation);
// the existing filenames feed its duplicate guard.
const newOpen = ref(false);
const copyOpen = ref(false);
const copyInitial = ref("");
const existingFilenames = computed(() => summaries.value.map((s) => s.filename));

function openNew() {
  if (!selectedTemplate.value) return;
  newOpen.value = true;
}

async function submitNew(filename: string) {
  // Open an unsaved view, set selection - persist happens on first Save.
  selectedDataFile.value = filename;
  await open(selectedTemplate.value, filename);
  newOpen.value = false;
  toast.success("workspace.storage.new.opened", [filename]);
  statusBar.setCreated(filename);
}

function openCopy() {
  if (!view.value?.saved) return;
  const stem = (view.value.datafile ?? "").replace(/\.meta\.json$/, "");
  copyInitial.value = stem ? `${stem}-copy` : "";
  copyOpen.value = true;
}

// Copy persists immediately (the point is to duplicate the file with a fresh
// id), then opens the new record. The backend owns the id/audit reset.
async function submitCopy(filename: string) {
  if (!selectedTemplate.value || !view.value?.datafile) return;
  try {
    await FormSvc.CopyForm(selectedTemplate.value, view.value.datafile, filename);
    selectedDataFile.value = filename;
    await open(selectedTemplate.value, filename);
    await refreshList();
    copyOpen.value = false;
    toast.success("workspace.storage.copy.copied", [filename]);
    statusBar.setCreated(filename);
  } catch (e) {
    toast.error("workspace.storage.copy.error", [backendErrMessage(e)]);
  }
}

// ── Save / Reset / Delete ────────────────────────────────────────────
// Save never reloads the full sidebar list. For an existing entry we
// patch its summary in place (Vue's Proxy re-renders just that one
// <StorageListItem>); for a brand-new entry we append the summary.
// The sub-label refresh is one EvaluateListOne IPC call regardless
// - the workspace updates `sidebarItems[df]` and Vue propagates it
// to the matching row via prop. Keeps the rest of the list (and its
// scroll position) untouched.
async function doSave() {
  if (!draft.value) return;
  const result = await save();
  if (result.ok) {
    const df = draft.value?.datafile ?? "?";
    toast.success("workspace.storage.save.success", [df]);
    statusBar.setSaved(df);
    patchSummary(df);
    await refreshSidebarItem(df);
    await refreshMarkdown();
  } else {
    toast.error("workspace.storage.save.error", [result.message ?? "?"]);
  }
}

// patchSummary mutates the matching sidebar entry in place from the
// just-saved view, so the row's title / tags / flag refresh without
// reassigning summaries.value (which thrashes the sidebar scroll).
// Vue's reactive Proxy detects per-property writes, so only the one
// <StorageListItem> with this filename re-renders. For a brand-new
// entry (no matching row yet) the summary is appended - the new
// <StorageListItem> mounts and loads its own sidebar expression.
function patchSummary(filename: string): void {
  if (!view.value) return;
  const tpl = view.value.template;
  const itemField = tpl?.item_field ?? "";
  const titleRaw = itemField ? view.value.values?.[itemField] : "";
  const nextTitle = typeof titleRaw === "string" && titleRaw.length > 0
    ? titleRaw
    : filename;
  const idx = summaries.value.findIndex((s) => s.filename === filename);
  if (idx < 0) {
    summaries.value.push({
      filename,
      meta: view.value.meta,
      title: nextTitle,
      expressionItems: {},
    });
    return;
  }
  const entry = summaries.value[idx];
  entry.meta = view.value.meta;
  entry.title = nextTitle;
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
    statusBar.setDeleted(filename);
    selectedDataFile.value = "";
    // Splice the row out in place - mirrors the save path's "never
    // reassign summaries" rule, so the rest of the list (and its
    // scroll position) stays untouched.
    const idx = summaries.value.findIndex((s) => s.filename === filename);
    if (idx >= 0) summaries.value.splice(idx, 1);
    dropSidebarItem(filename);
  } else {
    toast.error("workspace.storage.delete.error", [result.message ?? "?"]);
  }
}

// ── Preview slideouts ────────────────────────────────────────────────
// Markdown is rendered eagerly: refreshed when a saved form opens and
// after each successful save. HTML is derived from the markdown only
// when the HTML slideout is open (and re-derived if markdown changes
// while it's open).
const mdOpen = ref(false);
const htmlOpen = ref(false);
const markdown = ref("");
const html = ref("");
const markdownError = ref("");
const htmlError = ref("");

async function refreshMarkdown() {
  markdownError.value = "";
  if (!view.value?.saved || !draft.value?.template?.filename || !draft.value?.datafile) {
    markdown.value = "";
    return;
  }
  try {
    markdown.value = await RenderSvc.RenderMarkdown(
      draft.value.template.filename,
      draft.value.datafile,
    );
  } catch (err) {
    markdown.value = "";
    markdownError.value = backendErrMessage(err);
  }
}

async function refreshHtml() {
  htmlError.value = "";
  if (!markdown.value) {
    html.value = "";
    return;
  }
  try {
    html.value = await RenderSvc.RenderHTML(markdown.value);
  } catch (err) {
    html.value = "";
    htmlError.value = backendErrMessage(err);
  }
}

watch(
  () => [view.value?.saved, draft.value?.template?.filename, draft.value?.datafile] as const,
  () => { void refreshMarkdown(); },
  { immediate: true },
);

// HTML lazily - only when its slideout is open. Re-derive if either
// the open state flips on, or the underlying markdown changes while
// the slideout is open.
watch([htmlOpen, markdown], async ([open]) => {
  if (open) await refreshHtml();
  else html.value = "";
});

// formidable:// click interceptor for the HTML preview slideout.
// The rendered body can include `<a href="formidable://tpl.yaml:datafile">`
// (link fields). The webview can't resolve the custom scheme, so we
// catch the click here, route through the Nav service (it parses,
// validates, persists the selection, and emits nav:changed), and
// close the slideout - App.vue's global nav:changed listener flips
// to the target form.
const formidableLink = useFormidableLink();
async function onHtmlPreviewClick(e: MouseEvent) {
  // Walk up the click path to the nearest <a>. Anchors inside the
  // rendered prose can wrap inline content (img, span, code …) so a
  // direct e.target check isn't enough.
  let el = e.target as HTMLElement | null;
  while (el && el !== e.currentTarget) {
    if (el.tagName === "A") break;
    el = el.parentElement;
  }
  if (!el || el.tagName !== "A") return;
  const href = (el as HTMLAnchorElement).getAttribute("href") || "";
  if (!href.startsWith("formidable://")) return;
  e.preventDefault();
  const handled = await formidableLink.follow(href);
  if (handled) {
    // Slideout would obscure the new form view; close it so the
    // user lands on the freshly-loaded record cleanly.
    htmlOpen.value = false;
  }
}

// "Copy HTML" doesn't ship the in-app fragment - it asks the backend
// for a self-contained document (DOCTYPE + head + inlined CSS + body)
// so the result pastes cleanly into a .html file and renders the same.
// Wired as an async getter into CopyButton; the backend call runs on
// click, not on render.
async function fetchFullHtml(): Promise<string> {
  const tplName = draft.value?.template?.filename;
  const datafile = draft.value?.datafile;
  if (!tplName || !datafile || !view.value?.saved) return "";
  return await RenderSvc.RenderFullHTML(tplName, datafile);
}

// ── Topbar menu ──────────────────────────────────────────────────────
function toggleMetaSection() {
  const next = !(config.value?.show_meta_section ?? true);
  updateConfig({ show_meta_section: next });
}

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
  const tpl = selectedTemplate.value;
  if (!tpl) return;
  try {
    const path = await StorageSvc.TemplateStorageDir(tpl);
    if (!path) return;
    await SystemSvc.OpenExternal(path);
  } catch (e) {
    toast.error("workspace.templates.open_folder.error", [backendErrMessage(e)]);
  }
}

// Data → Reindex: force-rebuild this collection's index rows (search
// body, values, tags, facets, title) from disk, then re-read the list
// and re-run any active search so the sidebar reflects the rebuild.
// SSOT: the op-tracker (backend) decides whether a reindex is in flight, so a
// reload or another view reflects it; the local latch only covers the click
// gap before the backend's optrack:changed lands.
const { isRunning } = useActiveOps();
const reindexing = ref(false);
function reindexBusy(tpl: string): boolean {
  return reindexing.value || isRunning("index:rescan:" + tpl);
}
async function reindexCollection() {
  const tpl = selectedTemplate.value;
  if (!tpl || reindexBusy(tpl)) return;
  reindexing.value = true;
  try {
    await IndexSvc.RescanTemplate(tpl);
    await refreshList();
    if (searchResults.value !== null) await runSearch();
    toast.success("workspace.storage.reindex.success", [tpl]);
  } catch (e) {
    toast.error("workspace.storage.reindex.error", [backendErrMessage(e)]);
  } finally {
    reindexing.value = false;
  }
}

// ── CSV import / export dialogs ─────────────────────────────────────
// Wails-side CSV import/export is collection-independent at the backend
// (storage.ImportCsvRow + csv.Export both operate per-form). The
// io_collection_only profile flag opts back into the old Formidable
// rule that limited the dialog to enable_collection templates. The
// HTTP API's bulk-write surface stays gated on enable_collection in
// handler.go regardless of this flag.
const importOpen = ref(false);
const activeTemplateObj = computed(() => {
  const f = selectedTemplate.value;
  return f ? templateCache.value.get(f) ?? null : null;
});
const ioAllowed = computed(
  () => !config.value?.io_collection_only || !!activeTemplateObj.value?.enable_collection,
);

// Presentation mode: the record list is an ordered deck. Drag-to-reorder is
// only offered when nothing is filtering the list (a filtered subset would make
// "the order" ambiguous); the list still renders in sequence order either way.
const presentationMode = computed(() => !!activeTemplateObj.value?.presentation);
const reorderEnabled = computed(
  () =>
    presentationMode.value &&
    !hasActiveFilters.value &&
    searchResults.value === null,
);

function openImport() {
  if (!selectedTemplate.value || !ioAllowed.value) return;
  importOpen.value = true;
}

async function onImported(count: number) {
  if (count > 0) await refreshList();
}

const exportOpen = ref(false);
function openExport() {
  if (!selectedTemplate.value || !ioAllowed.value) return;
  exportOpen.value = true;
}

const queryOpen = ref(false);
function openQuery() {
  if (!selectedTemplate.value) return;
  queryOpen.value = true;
}

const graphOpen = ref(false);
function openGraph() {
  if (!selectedTemplate.value || !view.value) return;
  graphOpen.value = true;
}

const exportPdfOpen = ref(false);
const { status: pdfStatus } = usePDFActivation();
const pdfActive = computed(() => pdfStatus.value?.active === true);
function openExportPdf() {
  if (!selectedTemplate.value || !view.value?.saved) return;
  exportPdfOpen.value = true;
}

// Plugins attached to the Storage workspace receive the active
// template's filename as ctx.template - so a plugin like wikiwonder
// can scope its work to "this template" rather than enumerating
// every template in the catalog.
const { buildMenu: buildPluginsMenu } = useWorkspacePluginMenu(
  "storage",
  () => selectedTemplate.value ?? "",
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
        labelKey: "workspace.storage.save",
        combo: "Mod+S",
        disabled: !dirty.value,
        onClick: doSave,
      },
      {
        id: "reset",
        labelKey: "workspace.storage.reset",
        disabled: !dirty.value,
        onClick: reset,
      },
      { type: "separator", id: "sep-undo" },
      {
        id: "undo",
        labelKey: "workspace.storage.undo",
        combo: "Mod+Z",
        disabled: !canUndo.value,
        onClick: undo,
      },
      {
        id: "redo",
        labelKey: "workspace.storage.redo",
        combo: "Mod+Shift+Z",
        disabled: !canRedo.value,
        onClick: redo,
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
        disabled: !selectedTemplate.value,
        onClick: openStorageFolder,
      },
    ],
  },
  {
    type: "group",
    id: "entry",
    labelKey: "menu.storage",
    alwaysEnabled: true,
    items: [
      {
        id: "new-entry",
        labelKey: "workspace.storage.new_entry",
        combo: "Mod+N",
        disabled: !selectedTemplate.value,
        onClick: openNew,
      },
      {
        id: "delete-entry",
        labelKey: "workspace.storage.delete",
        combo: "Mod+D",
        disabled: !view.value?.saved,
        onClick: askDelete,
      },
      { type: "separator", id: "entry-sep" },
      {
        id: "toggle-meta",
        labelKey: "workspace.storage.toggle_meta",
        combo: "Mod+M",
        onClick: toggleMetaSection,
      },
      {
        id: "preview-markdown",
        labelKey: "workspace.storage.preview.markdown",
        combo: "Ctrl+Shift+M",
        allowWhenTyping: true,
        disabled: !view.value?.saved,
        onClick: () => { mdOpen.value = !mdOpen.value; },
      },
      {
        id: "preview-html",
        labelKey: "workspace.storage.preview.html",
        combo: "Ctrl+Shift+H",
        allowWhenTyping: true,
        disabled: !view.value?.saved,
        onClick: () => { htmlOpen.value = !htmlOpen.value; },
      },
    ],
  },
  {
    type: "group",
    id: "data",
    labelKey: "menu.data",
    alwaysEnabled: true,
    items: [
      {
        id: "import",
        labelKey: "menu.data.import",
        disabled: !selectedTemplate.value || !ioAllowed.value,
        onClick: openImport,
      },
      {
        id: "export",
        labelKey: "menu.data.export",
        disabled: !selectedTemplate.value || !ioAllowed.value,
        onClick: openExport,
      },
      {
        id: "query",
        labelKey: "menu.data.query",
        disabled: !selectedTemplate.value,
        onClick: openQuery,
      },
      {
        id: "graph",
        labelKey: "menu.data.graph",
        disabled: !selectedTemplate.value || !view.value,
        onClick: openGraph,
      },
      { type: "separator", id: "data-sep-reindex" },
      {
        id: "reindexCollection",
        labelKey: "menu.data.reindex",
        disabled: !selectedTemplate.value || reindexBusy(selectedTemplate.value),
        onClick: reindexCollection,
      },
      // PDF export is hidden entirely while the engine is inactive -
      // user activates it from the Information workspace. The
      // separator rides along so the menu doesn't show a dangling
      // divider when the entry is hidden.
      ...(pdfActive.value
        ? [
            { type: "separator", id: "data-sep" } as const,
            {
              id: "exportPdf",
              labelKey: "menu.data.export_pdf",
              disabled: !selectedTemplate.value || !view.value?.saved,
              onClick: openExportPdf,
            },
          ]
        : []),
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
        {{ t('workspace.storage.dirty_indicator') }}
      </Badge>
      <button
        v-if="view"
        class="tool-btn"
        :disabled="!canUndo"
        :title="t('workspace.storage.undo')"
        :aria-label="t('workspace.storage.undo')"
        @click="undo"
      >↶</button>
      <button
        v-if="view"
        class="tool-btn"
        :disabled="!canRedo"
        :title="t('workspace.storage.redo')"
        :aria-label="t('workspace.storage.redo')"
        @click="redo"
      >↷</button>
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
        v-if="config?.show_copy_button ?? true"
        class="tool-btn"
        :disabled="!view || !view.saved"
        @click="openCopy"
      >
        + {{ t('workspace.storage.copy') }}
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

  <SplitPane :initial="sidebarWidth" :sidebar-split="true">
    <template #sidebar>
      <div class="sidebar-title-row">
        <h2 class="sidebar-title">{{ t('workspace.storage.sidebar_title') }}</h2>
        <Popup placement="below" max-width="360px" teleport>
          <template #trigger="{ toggle, open }">
            <button
              type="button"
              class="tool-btn sidebar-relations-btn"
              :class="{ 'is-active': open }"
              :disabled="!canLinkRelations"
              :title="t('workspace.storage.relations.button')"
              @click="toggle"
            >{{ t('workspace.storage.relations.button') }}</button>
          </template>
          <RelationLinksPanel
            :template="selectedTemplate"
            :record-id="currentRecordId"
            :sidebar-items="sidebarRelationItems"
          />
        </Popup>
      </div>

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
          <FilteredCount :visible="visibleSummaries.length" :total="summaries.length" />
          <button
            v-if="presentationMode"
            type="button"
            class="tool-btn sidebar-normalize"
            :title="t('workspace.storage.presentation.normalize_hint')"
            @click="normalizeSequence"
          >{{ t('workspace.storage.presentation.normalize') }}</button>
        </div>

        <div v-if="ftsEnabled" class="sidebar-section">
          <StorageSearch v-model="searchQuery" />
        </div>

        <div v-if="usedFacets.length > 0" class="sidebar-toolbar">
          <StorageFacetFilter
            v-for="f in usedFacets"
            :key="f.key"
            :facet="f"
            :model-value="facetFilters[f.key] ?? ''"
            @update:model-value="(v: string) => setFacetFilter(f.key, v)"
          />
        </div>

        <div v-if="hasTagsField" class="sidebar-tag-row">
          <StorageTagFilter v-model="tagFilter" />
        </div>

        <div
          v-if="usedFacets.length > 0 || hasTagsField"
          class="sidebar-filter-clear-row"
        >
          <button
            type="button"
            class="tool-btn sidebar-filter-clear"
            :class="{ danger: hasActiveFilters, 'is-muted': !hasActiveFilters }"
            :disabled="!hasActiveFilters"
            :title="t('workspace.storage.facet_filter.clear')"
            :aria-label="t('workspace.storage.facet_filter.clear')"
            @click="clearFilters"
          >{{ t('workspace.storage.facet_filter.clear') }}</button>
        </div>
      </div>

      <div v-if="!selectedTemplate || listError || visibleSummaries.length === 0" class="sidebar-scroll">
        <p v-if="!selectedTemplate" class="muted small">
          {{ t('workspace.storage.no_template_selected') }}
        </p>
        <p v-else-if="listError" class="form-error small">{{ listError }}</p>
        <p v-else class="muted small">
          {{ ftsEnabled && searchResults !== null
            ? t('workspace.storage.search.empty')
            : t('workspace.storage.empty') }}
        </p>
      </div>

      <draggable
        v-else-if="reorderEnabled"
        class="sidebar-scroll"
        :model-value="visibleSummaries"
        :item-key="(s: FormSummary) => s.filename"
        handle=".dnd-handle"
        ghost-class="dnd-ghost"
        chosen-class="dnd-chosen"
        drag-class="dnd-drag"
        :animation="150"
        @change="onReorderChange"
      >
        <template #item="{ element: item }">
          <div class="sidebar-reorder-row">
            <span
              class="dnd-handle"
              :title="t('workspace.storage.field.drag_to_reorder')"
              @click.stop
            >⠿</span>
            <StorageListItem
              :summary="item"
              :expression="sidebarItems.get(item.filename) ?? null"
              :active="item.filename === selectedDataFile"
              :facets="facets"
              @pick="pickForm"
            />
          </div>
        </template>
      </draggable>

      <VirtualList
        v-else
        ref="virtualList"
        class="sidebar-scroll"
        :items="visibleSummaries"
        :item-key="(s: FormSummary) => s.filename"
        :active-key="selectedDataFile"
        v-slot="{ item }"
      >
        <StorageListItem
          :summary="item"
          :expression="sidebarItems.get(item.filename) ?? null"
          :active="item.filename === selectedDataFile"
          :facets="facets"
          @pick="pickForm"
        />
      </VirtualList>
    </template>

    <template #main>
      <p v-if="!selectedTemplate" class="workspace-empty">
        {{ t('workspace.storage.placeholder_main') }}
      </p>
      <p v-else-if="!view || !draft" class="workspace-empty">
        {{ t('workspace.storage.unselected') }}
      </p>

      <template v-else>
        <!-- Meta scaffold. Hidden via Mod+M (config.show_meta_section). -->
        <StorageMetaBlock
          v-if="config?.show_meta_section ?? true"
          :datafile="draft.datafile"
          :meta="draft.meta"
          :facets="facets"
          @facet-state-change="onFacetStateChange"
        />

        <StorageDataForm
          v-if="draft.template"
          :template="draft.template"
          :values="draft.values"
          :loop-groups="draft.loop_groups"
        />
      </template>
    </template>
  </SplitPane>

  <!-- Right-edge preview slideouts: teleported to #app-main so they
       span the entire workspace width (sidebar + main) up to the ribbon. -->
  <template v-if="view">
    <RightSlideout
      v-model:open="mdOpen"
      :title="t('workspace.storage.preview.markdown')"
      :handle-label="t('workspace.storage.preview.markdown_handle')"
      offset-top="var(--space-3)"
    >
      <template #header-actions>
        <CopyButton
          :text="markdown"
          :disabled="!markdown"
          title-key="workspace.storage.preview.copy_markdown"
          success-key="workspace.storage.preview.copied_markdown"
          error-key="workspace.storage.preview.copy_error"
          button-class="right-slideout-action"
        />
      </template>
      <pre v-if="markdown" class="preview-markdown">{{ markdown }}</pre>
      <div v-else-if="markdownError" class="preview-error">
        <p class="preview-error-title">{{ t('workspace.storage.preview.markdown_error') }}</p>
        <pre class="preview-error-body">{{ markdownError }}</pre>
      </div>
      <p v-else class="muted small">{{ t('workspace.storage.preview.markdown_empty') }}</p>
    </RightSlideout>
    <RightSlideout
      v-model:open="htmlOpen"
      :title="t('workspace.storage.preview.html')"
      :handle-label="t('workspace.storage.preview.html_handle')"
      offset-top="calc(var(--space-3) + var(--right-slideout-handle-h) + 1px)"
    >
      <template #header-actions>
        <CopyButton
          :text="fetchFullHtml"
          :disabled="!html"
          title-key="workspace.storage.preview.copy_html"
          success-key="workspace.storage.preview.copied_html"
          error-key="workspace.storage.preview.copy_error"
          button-class="right-slideout-action"
        />
      </template>
      <RenderedHtml
        v-if="html"
        class="preview-html formidable-prose"
        :html="html"
        @click="onHtmlPreviewClick"
      />
      <div v-else-if="htmlError" class="preview-error">
        <p class="preview-error-title">{{ t('workspace.storage.preview.html_error') }}</p>
        <pre class="preview-error-body">{{ htmlError }}</pre>
      </div>
      <p v-else class="muted small">{{ t('workspace.storage.preview.html_empty') }}</p>
    </RightSlideout>
  </template>

  <!-- New entry dialog -->
  <EntryNameDialog
    :open="newOpen"
    :title="t('workspace.storage.new.title')"
    :confirm-label="t('workspace.storage.new_entry')"
    :placeholder="t('workspace.storage.new.placeholder')"
    :existing-names="existingFilenames"
    @cancel="newOpen = false"
    @submit="submitNew"
  />

  <!-- Copy entry dialog -->
  <EntryNameDialog
    :open="copyOpen"
    :title="t('workspace.storage.copy.title')"
    :confirm-label="t('workspace.storage.copy')"
    :placeholder="t('workspace.storage.copy.placeholder')"
    :existing-names="existingFilenames"
    :initial-name="copyInitial"
    @cancel="copyOpen = false"
    @submit="submitCopy"
  />

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

  <!-- Unsaved-changes guard: shown when navigating away from a dirty
       form (entry / template switch, workspace switch, or app close). -->
  <UnsavedChangesDialog
    :open="leavePromptOpen"
    :title="t('workspace.storage.unsaved.title')"
    :message="t('workspace.storage.unsaved.message', [view?.datafile ?? ''])"
    :save-label="t('workspace.storage.unsaved.save')"
    :discard-label="t('workspace.storage.unsaved.discard')"
    :cancel-label="t('common.cancel')"
    @save="resolveLeave('save')"
    @discard="resolveLeave('discard')"
    @cancel="resolveLeave('cancel')"
  />

  <!-- Import dialog (CSV / Excel) -->
  <ImportDialog
    :open="importOpen"
    :template-filename="selectedTemplate"
    :template="activeTemplateObj"
    @close="importOpen = false"
    @imported="onImported"
  />

  <!-- Export dialog (CSV / Excel) -->
  <ExportDialog
    :open="exportOpen"
    :template-filename="selectedTemplate"
    :template="activeTemplateObj"
    @close="exportOpen = false"
  />

  <!-- Export PDF dialog -->
  <ExportPDFDialog
    :open="exportPdfOpen"
    :template-filename="selectedTemplate"
    :datafile="view?.datafile ?? ''"
    @close="exportPdfOpen = false"
  />

  <!-- Query dialog (read-only SELECT over indexed values) -->
  <QueryDialog
    :open="queryOpen"
    :template-filename="selectedTemplate"
    :template="activeTemplateObj"
    @close="queryOpen = false"
  />

  <!-- Datacore graph (live node-link view rooted at the selected record) -->
  <DatacoreGraphDialog
    :open="graphOpen"
    :template-filename="selectedTemplate"
    :record="view?.datafile ?? ''"
    @close="graphOpen = false"
  />
</template>

