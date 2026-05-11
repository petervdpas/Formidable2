<script setup lang="ts">
import { computed, provide, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Clipboard } from "@wailsio/runtime";
import SplitPane from "../components/SplitPane.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import RightSlideout from "../components/RightSlideout.vue";
import { SelectField, SwitchField } from "../components/fields";
import StorageListItem from "../components/StorageListItem.vue";
import StorageTagFilter from "../components/StorageTagFilter.vue";
import StorageFlagFilter from "../components/StorageFlagFilter.vue";
import StorageMetaBlock from "../components/StorageMetaBlock.vue";
import StorageDataForm from "../components/StorageDataForm.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useTemplates } from "../composables/useTemplates";
import { useFormView } from "../composables/useFormView";
import { useConfig } from "../composables/useConfig";
import { useToast } from "../composables/useToast";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useFormidableLink } from "../composables/useFormidableLink";
import { Service as FormSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { Service as ExpressionSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
import type { SidebarItem } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
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

const hasTagsField = computed(() => {
  const tpl = templateCache.value.get(selectedTemplate.value);
  return !!tpl?.fields?.some((f) => f.type === "tags");
});

const flagDefinitions = computed(() => {
  const tpl = templateCache.value.get(selectedTemplate.value);
  return tpl?.flag_definitions ?? [];
});

// LABEL → color lookup for the active template, used by the sidebar
// list to color each row's flag icon. Stays in sync as the user edits
// flag_definitions in the template editor.
const flagColorByLabel = computed(() => {
  const m = new Map<string, string>();
  for (const d of flagDefinitions.value) m.set(d.label, d.color);
  return m;
});

// Set/clear the flag on the active draft. Picking a state implies
// flagged=true; clearing wipes both fields. Keeps the legacy bool in
// sync so old consumers (sidebar list, exports) continue to work.
function onFlagStateChange(state: string) {
  if (!draft.value?.meta) return;
  draft.value.meta.flag_state = state;
  draft.value.meta.flagged = state !== "";
}

// ── Form list (sidebar) ──────────────────────────────────────────────
const summaries = ref<FormSummary[]>([]);
const listError = ref("");

// Per-row expression results, keyed by record filename. Populated by
// refreshExpressions after every list refresh when the user has
// `use_expressions` enabled and the active template has a
// sidebar_expression. Empty map means "show no sub-label" — either
// the engine returned ErrNoExpression or the toggle is off.
const expressionItems = ref<Map<string, SidebarItem>>(new Map());

async function refreshExpressions() {
  expressionItems.value = new Map();
  if (!selectedTemplate.value) return;
  if (!config.value?.use_expressions) return;
  try {
    const items = await ExpressionSvc.EvaluateSidebar(selectedTemplate.value);
    const next = new Map<string, SidebarItem>();
    for (const it of items) {
      if (it?.filename) next.set(it.filename, it);
    }
    expressionItems.value = next;
  } catch {
    // ErrNoExpression and any other failure mean "no sub-label" —
    // sidebar continues to render the title row unchanged.
    expressionItems.value = new Map();
  }
}

async function refreshList() {
  if (!selectedTemplate.value) {
    summaries.value = [];
    expressionItems.value = new Map();
    return;
  }
  listError.value = "";
  try {
    await FormSvc.EnsureFormDir(selectedTemplate.value);
    summaries.value = await FormSvc.ListForms(selectedTemplate.value);
    await refreshExpressions();
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
  } catch (err) {
    listError.value = String(err);
    summaries.value = [];
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

// Live-toggle: flipping use_expressions in Settings re-fetches
// without a template change. Cheap (one Wails call) and keeps the
// sidebar reactive so the user sees the effect immediately.
watch(() => config.value?.use_expressions, async () => {
  await refreshExpressions();
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
  },
  { immediate: true },
);

function pickForm(filename: string) {
  selectedDataFile.value = filename;
}

// ── Sidebar filters ─────────────────────────────────────────────────
// flagFilter: "" = no filter (show all); else a state LABEL — only
// forms whose meta.flag_state matches are kept.
const flagFilter = ref("");
const tagFilter = ref("");

// Reset the flag filter when the active template changes — the new
// template's flag_definitions may not include the previously-picked
// label, which would otherwise leave the sidebar mysteriously empty.
watch(selectedTemplate, () => {
  flagFilter.value = "";
});

const visibleSummaries = computed(() => {
  let out = summaries.value;
  if (flagFilter.value) {
    out = out.filter((s) => (s.meta?.flag_state ?? "") === flagFilter.value);
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
}

// ── Save / Reset / Delete ────────────────────────────────────────────
async function doSave() {
  if (!draft.value) return;
  const result = await save();
  if (result.ok) {
    toast.success("workspace.storage.save.success", [draft.value?.datafile ?? "?"]);
    await refreshList();
    await refreshMarkdown();
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

// Webviews block navigator.clipboard outside secure contexts; route
// through Wails' Clipboard runtime which calls into the native
// platform API.
async function copyToClipboard(text: string, successKey: string) {
  if (!text) return;
  try {
    await Clipboard.SetText(text);
    toast.success(successKey);
  } catch {
    toast.error("workspace.storage.preview.copy_error");
  }
}

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
async function copyFullHtml() {
  const tplName = draft.value?.template?.filename;
  const datafile = draft.value?.datafile;
  if (!tplName || !datafile || !view.value?.saved) return;
  try {
    const full = await RenderSvc.RenderFullHTML(tplName, datafile);
    await Clipboard.SetText(full);
    toast.success("workspace.storage.preview.copied_html");
  } catch {
    toast.error("workspace.storage.preview.copy_error");
  }
}

// ── Topbar menu ──────────────────────────────────────────────────────
function toggleMetaSection() {
  const next = !(config.value?.show_meta_section ?? true);
  updateConfig({ show_meta_section: next });
}

setTopbarMenu(() => [
  {
    type: "group",
    id: "file",
    labelKey: "menu.file",
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
          <span class="muted small">{{ summaries.length }}</span>
        </div>

        <div v-if="flagDefinitions.length > 0" class="sidebar-toolbar">
          <StorageFlagFilter
            v-model="flagFilter"
            :definitions="flagDefinitions"
          />
        </div>

        <StorageTagFilter v-if="hasTagsField" v-model="tagFilter" />
      </div>

      <div class="sidebar-scroll">
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
            :active="s.filename === selectedDataFile"
            :expression="expressionItems.get(s.filename)"
            :flag-color="flagColorByLabel.get(s.meta?.flag_state ?? '')"
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
          :flag-definitions="flagDefinitions"
          @flag-state-change="onFlagStateChange"
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
        <button
          type="button"
          class="right-slideout-action"
          :disabled="!markdown"
          :title="t('workspace.storage.preview.copy_markdown')"
          :aria-label="t('workspace.storage.preview.copy_markdown')"
          @click="copyToClipboard(markdown, 'workspace.storage.preview.copied_markdown')"
        >
          <i class="fa-solid fa-copy"></i>
        </button>
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
        <button
          type="button"
          class="right-slideout-action"
          :disabled="!html"
          :title="t('workspace.storage.preview.copy_html')"
          :aria-label="t('workspace.storage.preview.copy_html')"
          @click="copyFullHtml"
        >
          <i class="fa-solid fa-copy"></i>
        </button>
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
</template>

