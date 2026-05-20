<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import Badge from "../components/Badge.vue";
import CopyButton from "../components/CopyButton.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import RightSlideout from "../components/RightSlideout.vue";
import ImportCSVDialog from "../components/ImportCSVDialog.vue";
import ExportCSVDialog from "../components/ExportCSVDialog.vue";
import ExportPDFDialog from "../components/ExportPDFDialog.vue";
import { SelectField, SwitchField } from "../components/fields";
import FilteredCount from "../components/FilteredCount.vue";
import StorageListItem from "../components/StorageListItem.vue";
import StorageTagFilter from "../components/StorageTagFilter.vue";
import StorageFacetFilter from "../components/StorageFacetFilter.vue";
import StorageMetaBlock from "../components/StorageMetaBlock.vue";
import StorageDataForm from "../components/StorageDataForm.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useTemplates } from "../composables/useTemplates";
import { useFormView } from "../composables/useFormView";
import { useConfig } from "../composables/useConfig";
import { useToast } from "../composables/useToast";
import { useStatusBar } from "../composables/useStatusBar";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useWorkspacePluginMenu } from "../composables/useWorkspacePluginMenu";
import { useFormidableLink } from "../composables/useFormidableLink";
import { usePDFActivation } from "../composables/usePDFActivation";
import { Service as ExpressionSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
import { Service as FormSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { Service as SystemSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { backendErrMessage } from "../utils/backendError";
import { scrollToActiveRow } from "../utils/scrollToActiveRow";
import type { FormSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { FacetState } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { Result as ExpressionResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config, update: updateConfig } = useConfig();
// Storage picker shows only the templates the active profile has
// enabled in Settings → Templates — the filtered list comes pre-filtered
// from the backend (ConfigSvc.ListEnabledTemplates), so the picker
// always reflects the current profile's curation with no JS-side
// intersection.
const { enabledFilenames: templateFilenames, cache: templateCache } = useTemplates();
const { view, draft, dirty, open, close, save, reset, remove } = useFormView();
const toast = useToast();
const statusBar = useStatusBar();

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

// ── Form list (sidebar) ──────────────────────────────────────────────
const summaries = ref<FormSummary[]>([]);
const listError = ref("");

// Sidebar sub-label items keyed by datafile. The workspace owns this
// map and hands each row's entry down as a prop — collapses what was
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

// User-triggered refresh — same backend path as the watch-driven
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
// changed even when selectedTemplate didn't — re-read the list so
// the sidebar reflects upstream deletions/additions.
function onContextReloaded() {
  void refreshList();
}
onMounted(() => window.addEventListener("formidable:context-reloaded", onContextReloaded));
onBeforeUnmount(() => window.removeEventListener("formidable:context-reloaded", onContextReloaded));

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
    // `df` is the config-persisted stem; `draft.datafile` is the
    // actual on-disk filename (e.g. "projectstatus.meta.json").
    statusBar.setSelected(draft.value?.datafile ?? df);
  },
  { immediate: true },
);

// Scroll the active row into view exactly once per template context.
// Two cases to handle:
//  - In-app navigation: layout is settled, scrollToActiveRow runs
//    once and we're done.
//  - First startup: the splash blocks flex layout until after
//    refreshList resolves, so the container's clientHeight reads as
//    ~8px (just its padding). The math centers correctly only when
//    the container has a real height, so we observe it and retry
//    each resize until layout assigns it a sensible viewport.
// The flag gates one successful scroll per template so save / refresh
// / clicks don't yank the viewport, and the observer is also reset on
// template change.
const listScrollEl = ref<HTMLElement | null>(null);
let scrolledForTemplate: string | null = null;
let pendingScrollObserver: ResizeObserver | null = null;

function cancelPendingScroll() {
  pendingScrollObserver?.disconnect();
  pendingScrollObserver = null;
}

watch(selectedTemplate, () => {
  scrolledForTemplate = null;
  cancelPendingScroll();
});
onBeforeUnmount(cancelPendingScroll);

async function scrollActiveIntoView() {
  const tpl = selectedTemplate.value;
  const df = selectedDataFile.value;
  if (!tpl || !df) return;
  if (scrolledForTemplate === tpl) return;
  cancelPendingScroll();

  await nextTick();
  const container = listScrollEl.value;
  if (!container) return;

  // Heuristic: a container shorter than two rows of expected height
  // hasn't been sized by flex layout yet. ~140px ≈ two rows worth.
  const SETTLED_MIN_HEIGHT = 140;
  const attempt = (): boolean => {
    if (container.clientHeight < SETTLED_MIN_HEIGHT) return false;
    if (scrollToActiveRow(container, df)) {
      scrolledForTemplate = tpl;
      return true;
    }
    return false;
  };

  if (attempt()) return;

  // Container not yet sized — observe and retry on each resize until
  // layout settles (typically right after the splash dismisses).
  const ro = new ResizeObserver(() => {
    if (attempt()) {
      ro.disconnect();
      if (pendingScrollObserver === ro) pendingScrollObserver = null;
    }
  });
  ro.observe(container);
  pendingScrollObserver = ro;
}

const { follow: followFormidable } = useFormidableLink();

async function pickForm(filename: string) {
  const tpl = selectedTemplate.value;
  if (!tpl || !filename) return;
  await followFormidable(`formidable://${tpl}:${filename}`);
}

// ── Sidebar filters ─────────────────────────────────────────────────
// facetFilters: per-facet selected-label (or "" = no filter for that
// facet). A form passes when every active filter entry matches its
// meta.facets[key].selected with set=true.
const facetFilters = ref<Record<string, string>>({});
const tagFilter = ref("");

// Reset facet filters when the active template changes — the new
// template's facets may differ, which would otherwise leave the
// sidebar mysteriously empty.
watch(selectedTemplate, () => {
  facetFilters.value = {};
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

const visibleSummaries = computed(() => {
  let out = summaries.value;
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
  statusBar.setCreated(filename);
}

// ── Save / Reset / Delete ────────────────────────────────────────────
// Save never reloads the full sidebar list. For an existing entry we
// patch its summary in place (Vue's Proxy re-renders just that one
// <StorageListItem>); for a brand-new entry we append the summary.
// The sub-label refresh is one EvaluateListOne IPC call regardless
// — the workspace updates `sidebarItems[df]` and Vue propagates it
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
// entry (no matching row yet) the summary is appended — the new
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
    // Splice the row out in place — mirrors the save path's "never
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

async function refreshMarkdown() {
  if (!view.value?.saved || !draft.value?.template?.filename || !draft.value?.datafile) {
    markdown.value = "";
    return;
  }
  try {
    markdown.value = await RenderSvc.RenderMarkdown(
      draft.value.template.filename,
      draft.value.datafile,
    );
  } catch {
    markdown.value = "";
  }
}

async function refreshHtml() {
  if (!markdown.value) {
    html.value = "";
    return;
  }
  try {
    html.value = await RenderSvc.RenderHTML(markdown.value);
  } catch {
    html.value = "";
  }
}

watch(
  () => [view.value?.saved, draft.value?.template?.filename, draft.value?.datafile] as const,
  () => { void refreshMarkdown(); },
  { immediate: true },
);

// HTML lazily — only when its slideout is open. Re-derive if either
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
// close the slideout — App.vue's global nav:changed listener flips
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

// "Copy HTML" doesn't ship the in-app fragment — it asks the backend
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

// ── CSV import / export dialogs ─────────────────────────────────────
// Wails-side CSV import/export is collection-independent at the backend
// (storage.ImportCsvRow + csv.Export both operate per-form). The
// io_collection_only profile flag opts back into the old Formidable
// rule that limited the dialog to enable_collection templates. The
// HTTP API's bulk-write surface stays gated on enable_collection in
// handler.go regardless of this flag.
const importCsvOpen = ref(false);
const activeTemplateObj = computed(() => {
  const f = selectedTemplate.value;
  return f ? templateCache.value.get(f) ?? null : null;
});
const csvAllowed = computed(
  () => !config.value?.io_collection_only || !!activeTemplateObj.value?.enable_collection,
);

function openImportCsv() {
  if (!selectedTemplate.value || !csvAllowed.value) return;
  importCsvOpen.value = true;
}

async function onCsvImported(count: number) {
  if (count > 0) await refreshList();
}

const exportCsvOpen = ref(false);
function openExportCsv() {
  if (!selectedTemplate.value || !csvAllowed.value) return;
  exportCsvOpen.value = true;
}

const exportPdfOpen = ref(false);
const { status: pdfStatus } = usePDFActivation();
const pdfActive = computed(() => pdfStatus.value?.active === true);
function openExportPdf() {
  if (!selectedTemplate.value || !view.value?.saved) return;
  exportPdfOpen.value = true;
}

// Plugins attached to the Storage workspace receive the active
// template's filename as ctx.template — so a plugin like wikiwonder
// can scope its work to "this template" rather than enumerating
// every template in the catalog.
const { buildMenu: buildPluginsMenu } = useWorkspacePluginMenu(
  "storage",
  () => (selectedTemplate.value ? { template: selectedTemplate.value } : {}),
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
        id: "importCsv",
        labelKey: "menu.data.import",
        disabled: !selectedTemplate.value || !csvAllowed.value,
        onClick: openImportCsv,
      },
      {
        id: "exportCsv",
        labelKey: "menu.data.export",
        disabled: !selectedTemplate.value || !csvAllowed.value,
        onClick: openExportCsv,
      },
      // PDF export is hidden entirely while the engine is inactive —
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

  <SplitPane :initial="sidebarWidth" :sidebar-split="true">
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
          <FilteredCount :visible="visibleSummaries.length" :total="summaries.length" />
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
          <button
            type="button"
            class="tool-btn sidebar-filter-clear"
            :class="{ danger: hasActiveFilters, 'is-muted': !hasActiveFilters }"
            :disabled="!hasActiveFilters"
            :title="t('workspace.storage.facet_filter.clear')"
            :aria-label="t('workspace.storage.facet_filter.clear')"
            @click="clearFilters"
          >×</button>
        </div>
      </div>

      <div ref="listScrollEl" class="sidebar-scroll">
        <p v-if="!selectedTemplate" class="muted small">
          {{ t('workspace.storage.no_template_selected') }}
        </p>
        <p v-else-if="listError" class="form-error small">{{ listError }}</p>
        <p v-else-if="visibleSummaries.length === 0" class="muted small">
          {{ t('workspace.storage.empty') }}
        </p>

        <ul v-else class="form-list">
          <StorageListItem
            v-for="s in visibleSummaries"
            :key="s.filename"
            :summary="s"
            :expression="sidebarItems.get(s.filename) ?? null"
            :active="s.filename === selectedDataFile"
            :facets="facets"
            @pick="pickForm"
          />
        </ul>
      </div>
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
      <div
        v-if="html"
        class="preview-html formidable-prose"
        v-html="html"
        @click="onHtmlPreviewClick"
      />
      <p v-else class="muted small">{{ t('workspace.storage.preview.html_empty') }}</p>
    </RightSlideout>
  </template>

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

  <!-- Import CSV dialog -->
  <ImportCSVDialog
    :open="importCsvOpen"
    :template-filename="selectedTemplate"
    :template="activeTemplateObj"
    @close="importCsvOpen = false"
    @imported="onCsvImported"
  />

  <!-- Export CSV dialog -->
  <ExportCSVDialog
    :open="exportCsvOpen"
    :template-filename="selectedTemplate"
    :template="activeTemplateObj"
    @close="exportCsvOpen = false"
  />

  <!-- Export PDF dialog -->
  <ExportPDFDialog
    :open="exportPdfOpen"
    :template-filename="selectedTemplate"
    :datafile="view?.datafile ?? ''"
    @close="exportPdfOpen = false"
  />
</template>

