<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import SplitPane from "../components/SplitPane.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import CodeEditor from "../components/CodeEditor.vue";
import PluginCommandRow from "../components/PluginCommandRow.vue";
import PluginResultPanel from "../components/PluginResultPanel.vue";
import FieldEditModal from "../components/FieldEditModal.vue";
import FormFieldRow from "../components/form-fields/FormFieldRow.vue";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { getFieldTypeDef } from "../types/field-types";
import {
  Service as PluginSvc,
  Command,
  RunResultDTO,
  type ListResult,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { Service as DialogSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dialog";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
  TextareaField,
  SelectField,
} from "../components/fields";
import { useRestartGate } from "../composables/useRestartGate";
import { useToast } from "../composables/useToast";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { usePlugins, isValidPluginID } from "../composables/usePlugins";
import { usePluginEditor, isWidget } from "../composables/usePluginEditor";
import {
  setGlobalPluginRunning,
  useGlobalPluginRun,
  cancelGlobalPluginRun,
} from "../composables/useGlobalPluginRun";
import ProgressBarWidget from "../components/widgets/ProgressBarWidget.vue";
import StatusMessageWidget from "../components/widgets/StatusMessageWidget.vue";
import {
  Widget,
  Kind as WidgetKind,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const toast = useToast();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const {
  plugins,
  selectedID,
  selectedPlugin,
  workspaceIDs,
  refresh,
  create: createPlugin,
  remove,
  exportArchive,
  importArchive,
} = usePlugins();

// Manifest.workspaces is a string[] of attachment targets. The
// section renders one toggle per known workspace; the model coerces
// missing/null into an empty array so older manifests load cleanly.
function isWorkspaceAttached(ws: string): boolean {
  return (draftManifest.value?.workspaces ?? []).includes(ws);
}
function setWorkspaceAttached(ws: string, on: boolean) {
  if (!draftManifest.value) return;
  const cur = draftManifest.value.workspaces ?? [];
  if (on) {
    if (cur.includes(ws)) return;
    draftManifest.value.workspaces = [...cur, ws];
  } else {
    draftManifest.value.workspaces = cur.filter((w) => w !== ws);
  }
}

const { draftManifest, draftSource, draftForm, dirty, save, reset } = usePluginEditor();

// Pull global running/stopping state into the workspace so the inline
// Run modal can disable buttons / show Stop while the run is in
// flight. setGlobalPluginRunning is what flips them.
const { running: globalRunning, stopping: globalStopping } = useGlobalPluginRun();

async function stopGlobalRun() {
  await cancelGlobalPluginRun();
}

// Tabs below the Manifest section. Lua Source first (where the
// most editing happens); Commands edits the manifest's command
// list; Form Editor is a placeholder until the visual builder
// lands in the next slice.
type PluginTab = "source" | "commands" | "form";
const activeTab = ref<PluginTab>("source");

// ── Refresh ──────────────────────────────────────────────────────────
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
  if (!draftManifest.value || !selectedID.value) return;
  const r = await save();
  if (r.ok) {
    toast.success("workspace.plugins.save_success", [
      draftManifest.value?.name ?? selectedID.value,
    ]);
    return;
  }
  if (r.reason === "exception") {
    toast.error("workspace.plugins.save_error", [r.message]);
  }
}

function doReset() {
  reset();
}

// ── Create ───────────────────────────────────────────────────────────
const createOpen = ref(false);
const createInput = ref("");
const createError = ref<string>("");

function openCreate() {
  createInput.value = "";
  createError.value = "";
  createOpen.value = true;
}

async function submitCreate() {
  const id = createInput.value.trim();
  if (!isValidPluginID(id)) {
    createError.value = t("workspace.plugins.create.invalid");
    return;
  }
  const r = await createPlugin(id);
  if (r.ok) {
    toast.success("workspace.plugins.create.success", [id]);
    createOpen.value = false;
    return;
  }
  if (r.code === "exists") {
    createError.value = t("workspace.plugins.create.exists");
    return;
  }
  if (r.code === "invalid") {
    createError.value = t("workspace.plugins.create.invalid");
    return;
  }
  createError.value = t("workspace.plugins.create.error", [r.message ?? "?"]);
}

// ── Delete ───────────────────────────────────────────────────────────
const deleteOpen = ref(false);

function openDelete() {
  if (!selectedID.value) return;
  deleteOpen.value = true;
}

const deleteName = computed(() => {
  return draftManifest.value?.name || selectedID.value;
});

async function confirmDelete() {
  const id = selectedID.value;
  deleteOpen.value = false;
  if (!id) return;
  const r = await remove(id);
  if (r.ok) {
    toast.success("workspace.plugins.delete.success", [id]);
  } else {
    toast.error("workspace.plugins.delete.error", [r.message ?? "?"]);
  }
}

// ── Export / Import archive ─────────────────────────────────────────
// Mirrors the PDF covers archive flow: native save/open dialogs gate
// the file path, the backend bundles or unpacks the zip, and an
// existing-target on import opens a ConfirmDialog before retrying
// with overwrite=true.
const zipFilters = computed(() => [
  { displayName: t('workspace.plugins.archive.filter.zip'), pattern: '*.zip' },
]);

async function onExport() {
  if (!selectedID.value) return;
  const id = selectedID.value;
  let picked = "";
  try {
    picked = await DialogSvc.ChooseSaveFile(`${id}.zip`, zipFilters.value);
  } catch {
    return;
  }
  if (!picked) return;
  const r = await exportArchive(id, picked);
  if (!r.ok) {
    toast.error("workspace.plugins.archive.export.error", [r.message]);
    return;
  }
  toast.success("workspace.plugins.archive.export.success", [r.zipPath]);
}

const importOverwriteOpen = ref(false);
const importPendingPath = ref<string>("");
const importPendingName = ref<string>("");

async function onImport() {
  let picked = "";
  try {
    picked = await DialogSvc.ChooseFile(zipFilters.value);
  } catch {
    return;
  }
  if (!picked) return;
  await runImport(picked, false);
}

async function runImport(zipPath: string, overwrite: boolean) {
  const r = await importArchive(zipPath, overwrite);
  if (r.ok) {
    toast.success(
      r.overwritten
        ? "workspace.plugins.archive.import.success_overwrite"
        : "workspace.plugins.archive.import.success",
      [r.id],
    );
    if (r.id) selectedID.value = r.id;
    return;
  }
  if (r.code === "exists") {
    importPendingPath.value = zipPath;
    importPendingName.value = zipPath.split(/[\\/]/).pop()?.replace(/\.zip$/i, "") ?? zipPath;
    importOverwriteOpen.value = true;
    return;
  }
  if (r.code === "older_version") {
    toast.error("workspace.plugins.archive.import.older_version", [r.message]);
    return;
  }
  toast.error("workspace.plugins.archive.import.error", [r.message]);
}

async function confirmImportOverwrite() {
  const path = importPendingPath.value;
  importOverwriteOpen.value = false;
  importPendingPath.value = "";
  if (path) await runImport(path, true);
}

function cancelImportOverwrite() {
  importOverwriteOpen.value = false;
  importPendingPath.value = "";
  importPendingName.value = "";
}

// ── Run dialog ───────────────────────────────────────────────────────
const runOpen = ref(false);
// Latest run result keyed by command id so re-opening the modal
// shows the previous output until the user runs again.
const runResults = ref<Record<string, RunResultDTO>>({});
const runningCmd = ref<string>("");


// Live form values bound to the FormFieldRow inputs in the Run
// modal. Whatever the user types ends up here, and we pass a clone
// as the `ctx` argument to every Lua call so scripts can read
// `ctx.<field.key>`. Re-seeded on plugin selection (or when the
// form schema changes) using each field's stored default + the
// per-type fallback from field-types.ts.
const runValues = ref<Record<string, unknown>>({});

function initialRunValues(entries: Array<Field | Widget>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const e of entries) {
    if (isWidget(e)) continue;
    const f = e;
    if (!f.key) continue;
    if (f.default !== undefined && f.default !== null) {
      out[f.key] = f.default;
      continue;
    }
    const def = getFieldTypeDef(f.type)?.defaultValue?.();
    out[f.key] = def !== undefined ? def : "";
  }
  return out;
}

watch(
  () => draftForm.value,
  (entries) => {
    runValues.value = initialRunValues(entries ?? []);
  },
  { immediate: true, deep: true },
);

async function openRun() {
  if (!selectedPlugin.value) return;
  // Pre-populate the form from the plugin's KV bag so the user
  // sees whatever they entered last session. Keys come from any
  // Field entries (widgets don't carry values).
  const entries = draftForm.value ?? [];
  const keys = entries
    .filter((e): e is Field => !isWidget(e))
    .map((f) => f.key)
    .filter((k): k is string => !!k);
  if (selectedPlugin.value && keys.length > 0) {
    try {
      const saved = await PluginSvc.LoadFormValues(
        selectedPlugin.value.id,
        keys,
      );
      const merged: Record<string, unknown> = { ...runValues.value };
      for (const k of keys) {
        if (saved && saved[k] !== undefined) {
          merged[k] = saved[k];
        }
      }
      runValues.value = merged;
    } catch {
      /* fall back to whatever the watcher seeded */
    }
  }
  runOpen.value = true;
}

// Manifests written by older builds may not carry run_mode; treat
// missing/empty as "modal". The selector ensures fresh manifests
// always serialize "modal" or "form".
const runMode = computed(
  () => (selectedPlugin.value?.manifest.run_mode || "modal") as "modal" | "form",
);

// Commands split by visibility rule: in form mode only the
// form_button commands are surfaced (as inline buttons inside
// the form area); in modal mode only non-form-button commands
// appear (as the existing cards). A command flagged form_button
// in a modal-mode plugin is intentionally hidden — author opt-in.
const visibleCommands = computed(() => {
  const all = selectedPlugin.value?.manifest.commands ?? [];
  if (runMode.value === "form") {
    return all.filter((c) => c.form_button);
  }
  return all.filter((c) => !c.form_button);
});

// Manifest description rendered through goldmark (RenderHTML) so
// plugin authors can use Markdown — bold, links, lists, code spans —
// in the run-modal callout. We re-render whenever the draft text
// changes so the live editor reflects the formatted output.
const descriptionHTML = ref<string>("");
watch(
  () => draftManifest.value?.description ?? "",
  async (md) => {
    if (!md.trim()) {
      descriptionHTML.value = "";
      return;
    }
    try {
      descriptionHTML.value = await RenderSvc.RenderHTML(md);
    } catch {
      // Fallback to plain text on render failure so the user still
      // sees their description rather than nothing.
      descriptionHTML.value = md;
    }
  },
  { immediate: true },
);

async function runCommand(p: ListResult, cmd: Command) {
  runningCmd.value = cmd.id;
  setGlobalPluginRunning(true);
  try {
    // ctx is empty in modal mode; in form mode the user-filled
    // form values flow into the Lua function so scripts read
    // ctx.<field-key> directly.
    const ctx =
      runMode.value === "form"
        ? { ...runValues.value }
        : ({} as Record<string, unknown>);
    // Persist form values to the plugin's KV bag (keyed by field
    // id) before running, so they survive restarts AND so Lua
    // scripts see them via formidable.kv.get(fieldKey). Modal
    // mode skips this — there are no form values to save.
    if (runMode.value === "form") {
      try {
        await PluginSvc.SaveFormValues(p.id, { ...runValues.value });
      } catch {
        /* persistence is best-effort; don't block the run */
      }
    }
    const res = await PluginSvc.Run(p.id, cmd.id, ctx);
    runResults.value[cmd.id] = res;
    if (res.kind === "busy") {
      toast.warn(res.message || "plugin: another command is currently running");
    }
    // Dispatch any formidable.toast.* events the script emitted.
    // useToast accepts plain text, so the message goes through
    // verbatim — no i18n key resolution.
    for (const ev of res.toasts ?? []) {
      const fn = toast[ev.level as "info" | "success" | "warn" | "error"];
      if (fn) fn(ev.message);
    }
    // log_as_toast on the command turns each formidable.log.* line
    // into a toast as well. Lines arrive as "[level] message"; we
    // parse the prefix to pick a matching toast variant (debug → info).
    if (cmd.log_as_toast) {
      for (const line of res.logLines ?? []) {
        const m = /^\[(\w+)\]\s*(.*)$/.exec(line);
        const level = (m?.[1] ?? "info").toLowerCase();
        const msg = m?.[2] ?? line;
        const variant: "info" | "success" | "warn" | "error" =
          level === "warn" ? "warn"
          : level === "error" ? "error"
          : "info";
        toast[variant](msg);
      }
    }
  } catch (err) {
    runResults.value[cmd.id] = new RunResultDTO({
      kind: "runtime_error",
      message: String(err),
    });
  } finally {
    runningCmd.value = "";
    setGlobalPluginRunning(false);
  }
}

// ── Form-editor field operations ─────────────────────────────────────
// The Form Editor tab manages draftForm: an array of Field objects
// matching the template-side schema (so the same FormFieldRenderer
// can later render plugin forms). Add/edit reuse FieldEditModal;
// delete walks behind a confirm dialog like the template fields do.
const fieldEditOpen = ref(false);
const fieldEditIndex = ref<number>(-1);
const fieldEditTarget = ref<Field | null>(null);
const fieldEditIsNew = ref(false);

function openFieldEdit(idx: number) {
  const entry = draftForm.value[idx];
  // Edit button is only rendered for Field rows; defensively bail
  // if invoked on a widget so the modal doesn't get fed garbage.
  if (!entry || isWidget(entry)) return;
  fieldEditIndex.value = idx;
  fieldEditTarget.value = entry;
  fieldEditIsNew.value = false;
  fieldEditOpen.value = true;
}

function openFieldAdd() {
  fieldEditIndex.value = -1;
  fieldEditTarget.value = null;
  fieldEditIsNew.value = true;
  fieldEditOpen.value = true;
}

function applyFieldEdit(updated: Field) {
  // Looper synthesis is template-specific; plugins don't use loops
  // in their input forms, so we just append (create) or replace
  // (edit). Type guards in FieldEditModal still keep the user from
  // picking unsupported types.
  const list = draftForm.value ?? [];
  if (fieldEditIsNew.value) {
    draftForm.value = [...list, updated];
  } else if (fieldEditIndex.value >= 0) {
    draftForm.value = [
      ...list.slice(0, fieldEditIndex.value),
      updated,
      ...list.slice(fieldEditIndex.value + 1),
    ];
  }
  fieldEditOpen.value = false;
  fieldEditTarget.value = null;
  fieldEditIndex.value = -1;
  fieldEditIsNew.value = false;
}

const fieldDeleteOpen = ref(false);
const fieldDeleteIndex = ref<number>(-1);

function askDeleteField(idx: number) {
  fieldDeleteIndex.value = idx;
  fieldDeleteOpen.value = true;
}

const fieldDeleteName = computed(() => {
  const e = draftForm.value[fieldDeleteIndex.value];
  if (!e) return "";
  if (isWidget(e)) return e.label || e.id;
  return e.label || e.key || "";
});

function confirmDeleteField() {
  const idx = fieldDeleteIndex.value;
  fieldDeleteOpen.value = false;
  fieldDeleteIndex.value = -1;
  if (idx < 0) return;
  const list = draftForm.value ?? [];
  draftForm.value = [...list.slice(0, idx), ...list.slice(idx + 1)];
}

// ── Form editor: widget helpers ──────────────────────────────────────
// draftForm holds fields AND widgets in one ordered list (form.json's
// shape). The draggable in the template binds straight to draftForm,
// so reorder is "drop into array at index N" — no separate widget
// state, no Order field.
function nextWidgetID(prefix: string): string {
  const used = new Set(
    (draftForm.value ?? []).filter(isWidget).map((w) => w.id),
  );
  let n = 1;
  while (used.has(`${prefix}${n}`)) n++;
  return `${prefix}${n}`;
}

function addWidget(kind: WidgetKind) {
  const id = nextWidgetID(kind === WidgetKind.KindProgressBar ? "bar" : "msg");
  const w = new Widget({ id, kind, label: "" });
  draftForm.value = [...(draftForm.value ?? []), w];
}

// ── Commands list editing ────────────────────────────────────────────
function addCommand() {
  if (!draftManifest.value) return;
  const next = draftManifest.value.commands
    ? [...draftManifest.value.commands]
    : [];
  next.push(new Command({ id: "", label: "", fn: "" }));
  draftManifest.value.commands = next;
}

function removeCommand(idx: number) {
  if (!draftManifest.value || !draftManifest.value.commands) return;
  const next = [...draftManifest.value.commands];
  next.splice(idx, 1);
  draftManifest.value.commands = next;
}

// ── Topbar menu ──────────────────────────────────────────────────────
// Save + Discard live as buttons on the right side of the topbar
// (see template) — same shape Storage uses. The File menu only
// keeps Refresh now; everything else is in the Plugin menu.
setTopbarMenu(() => [
  {
    type: "group",
    id: "file",
    labelKey: "menu.file",
    items: [
      {
        id: "refresh",
        labelKey: "common.refresh",
        onClick: doRefresh,
      },
    ],
  },
  {
    type: "group",
    id: "plugin",
    labelKey: "menu.plugin",
    items: [
      {
        id: "create",
        labelKey: "menu.plugin.create",
        onClick: openCreate,
      },
      {
        id: "delete",
        labelKey: "menu.plugin.delete",
        disabled: !selectedID.value,
        onClick: openDelete,
      },
      { type: "separator" },
      {
        id: "import",
        labelKey: "menu.plugin.import",
        onClick: onImport,
      },
      {
        id: "export",
        labelKey: "menu.plugin.export",
        disabled: !selectedID.value,
        onClick: onExport,
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
        {{ t('workspace.plugins.dirty_indicator') }}
      </Badge>
      <button
        v-if="selectedPlugin"
        class="tool-btn success tool-btn--icon"
        :disabled="(selectedPlugin.manifest.commands?.length ?? 0) === 0"
        :title="t('menu.plugin.run')"
        @click="openRun"
      >
        <i class="fa-solid fa-play"></i>
        <span>{{ t('workspace.plugins.run') }}</span>
      </button>
      <button
        v-if="selectedPlugin"
        class="tool-btn danger"
        :disabled="!dirty"
        @click="doReset"
      >
        {{ t('workspace.plugins.reset') }}
      </button>
      <button
        v-if="selectedPlugin"
        class="tool-btn primary"
        :disabled="!dirty"
        @click="doSave"
      >
        {{ t('workspace.plugins.save') }}
      </button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.plugins.sidebar_title') }}</h2>
      <p v-if="plugins.length === 0" class="muted small" v-html="t('workspace.plugins.empty_side', ['plugins/'])"></p>
      <ul v-else class="sidebar-list">
        <li
          v-for="p in plugins"
          :key="p.id"
          :class="['sidebar-row', 'sidebar-row--stack', { active: p.id === selectedID }]"
          @click="selectedID = p.id"
        >
          <span class="plugin-name">{{ p.manifest.name || p.id }}</span>
          <span class="plugin-meta">
            <Badge class="small">{{ p.id }}</Badge>
            <span class="muted small">v{{ p.manifest.version }}</span>
          </span>
        </li>
      </ul>
    </template>

    <template #main>
      <p
        v-if="!selectedPlugin || !draftManifest"
        class="workspace-empty"
        v-html="t('workspace.plugins.empty_main')"
      ></p>

      <template v-else>
        <div class="workspace-heading-row">
          <h1 class="workspace-heading">{{ draftManifest.name || selectedID }}</h1>
          <Badge variant="accent">{{ selectedID }}</Badge>
        </div>

        <FormSection :title="t('workspace.plugins.manifest.title')">
          <FormRow :label="t('workspace.plugins.manifest.name')">
            <TextField v-model="draftManifest.name" />
          </FormRow>
          <FormRow :label="t('workspace.plugins.manifest.version')">
            <TextField v-model="draftManifest.version" />
          </FormRow>
          <FormRow :label="t('workspace.plugins.manifest.author')">
            <TextField v-model="draftManifest.author" />
          </FormRow>
          <FormRow :label="t('workspace.plugins.manifest.description')">
            <TextareaField v-model="draftManifest.description" :rows="3" />
          </FormRow>
        </FormSection>

        <FormSection
          :title="t('workspace.plugins.workspaces.title')"
          :subtitle="t('workspace.plugins.workspaces.subtitle')"
          collapsible
          default-collapsed
        >
          <FormSwitchRow
            v-for="ws in workspaceIDs"
            :key="ws"
            :model-value="isWorkspaceAttached(ws)"
            @update:model-value="(v: boolean) => setWorkspaceAttached(ws, v)"
            :label="t(`ribbon.${ws}`)"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </FormSection>

        <FormSection
          :title="t('workspace.plugins.behavior.title')"
          :subtitle="t('workspace.plugins.behavior.subtitle')"
          collapsible
          default-collapsed
        >
          <FormRow
            :label="t('workspace.plugins.manifest.run_mode')"
            :description="t('workspace.plugins.manifest.run_mode_help')"
          >
            <SelectField
              :model-value="draftManifest.run_mode || 'modal'"
              @update:model-value="(v: string) => (draftManifest && (draftManifest.run_mode = v))"
              :options="[
                { value: 'modal', label: t('workspace.plugins.run_mode.modal') },
                { value: 'form',  label: t('workspace.plugins.run_mode.form')  },
              ]"
            />
          </FormRow>
          <FormSwitchRow
            v-model="draftManifest.debug"
            :label="t('workspace.plugins.manifest.debug')"
            :description="t('workspace.plugins.manifest.debug_help')"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
          <FormSwitchRow
            v-model="draftManifest.requires_internal_server"
            :label="t('workspace.plugins.manifest.requires_internal_server')"
            :description="t('workspace.plugins.manifest.requires_internal_server_help')"
            :on-label="t('common.on')"
            :off-label="t('common.off')"
          />
        </FormSection>

        <nav class="tabs" role="tablist">
          <button
            type="button"
            role="tab"
            :class="['tab', { active: activeTab === 'source' }]"
            :aria-selected="activeTab === 'source'"
            @click="activeTab = 'source'"
          >
            {{ t('workspace.plugins.tab.source') }}
          </button>
          <button
            type="button"
            role="tab"
            :class="['tab', { active: activeTab === 'commands' }]"
            :aria-selected="activeTab === 'commands'"
            @click="activeTab = 'commands'"
          >
            {{ t('workspace.plugins.tab.commands') }}
          </button>
          <button
            type="button"
            role="tab"
            :class="['tab', { active: activeTab === 'form' }]"
            :aria-selected="activeTab === 'form'"
            @click="activeTab = 'form'"
          >
            {{ t('workspace.plugins.tab.form') }}
          </button>
        </nav>

        <section v-show="activeTab === 'source'" class="tab-pane">
          <div class="plugin-source">
            <CodeEditor
              v-model="draftSource"
              lang="lua"
              :height="420"
              :title="selectedPlugin ? `${selectedPlugin.manifest.name || selectedPlugin.id} • ${t('workspace.plugins.tab.source')}` : ''"
            />
            <p class="muted small">{{ t('workspace.plugins.source.help') }}</p>
          </div>
        </section>

        <section v-show="activeTab === 'commands'" class="tab-pane">
          <p v-if="!draftManifest.commands || draftManifest.commands.length === 0" class="muted small">
            {{ t('workspace.plugins.commands.empty') }}
          </p>
          <ul v-else class="cmd-rows">
            <PluginCommandRow
              v-for="(c, i) in draftManifest.commands"
              :key="i"
              :command="c"
              @delete="removeCommand(i)"
            />
          </ul>
          <div class="cmd-add-row">
            <button class="tool-btn" type="button" @click="addCommand">
              + {{ t('workspace.plugins.commands.add') }}
            </button>
          </div>
        </section>

        <section v-show="activeTab === 'form'" class="tab-pane">
          <p v-if="draftForm.length === 0" class="muted small">
            {{ t('workspace.plugins.form.empty') }}
          </p>
          <draggable
            v-else
            v-model="draftForm"
            tag="ul"
            class="field-rows"
            handle=".dnd-handle"
            :animation="150"
            ghost-class="dnd-ghost"
            chosen-class="dnd-chosen"
            drag-class="dnd-drag"
            :item-key="(e: Field | Widget) => isWidget(e) ? `w:${e.id}` : `f:${e.key}`"
          >
            <template #item="{ element: entry, index: i }">
              <li
                v-if="isWidget(entry)"
                class="field-row field-row-widget"
                :data-type="entry.kind"
              >
                <span class="dnd-handle" aria-hidden="true">☰</span>
                <span class="field-row-label">{{ entry.label || entry.id }}</span>
                <span class="field-row-type">({{ entry.kind.toUpperCase() }})</span>
                <span class="field-row-spacer"></span>
                <div class="field-row-actions">
                  <button
                    type="button"
                    class="field-action-btn delete"
                    @click="askDeleteField(i)"
                  >
                    {{ t('workspace.plugins.commands.delete') }}
                  </button>
                </div>
              </li>
              <li
                v-else
                class="field-row"
                :data-type="entry.type"
              >
                <span class="dnd-handle" aria-hidden="true">☰</span>
                <span class="field-row-label">{{ entry.label || entry.key || `(field ${i + 1})` }}</span>
                <span class="field-row-type">({{ (entry.type || '').toUpperCase() }})</span>
                <span class="field-row-spacer"></span>
                <div class="field-row-actions">
                  <button
                    type="button"
                    class="field-action-btn edit"
                    @click="openFieldEdit(i)"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    class="field-action-btn delete"
                    @click="askDeleteField(i)"
                  >
                    {{ t('workspace.plugins.commands.delete') }}
                  </button>
                </div>
              </li>
            </template>
          </draggable>
          <div class="form-add-row">
            <button class="tool-btn" type="button" @click="openFieldAdd">
              + {{ t('workspace.plugins.form.add') }}
            </button>
            <button
              class="tool-btn"
              type="button"
              @click="addWidget(WidgetKind.KindProgressBar)"
            >
              + {{ t('workspace.plugins.form.add_progressbar') }}
            </button>
            <button
              class="tool-btn"
              type="button"
              @click="addWidget(WidgetKind.KindStatusMessage)"
            >
              + {{ t('workspace.plugins.form.add_statusmessage') }}
            </button>
          </div>
        </section>
      </template>
    </template>
  </SplitPane>

  <!-- Create modal -->
  <Modal
    :open="createOpen"
    :title="t('workspace.plugins.create.title')"
    @close="createOpen = false"
  >
    <label class="dialog-row">
      <span class="dialog-row-label">{{ t('workspace.plugins.create.label') }}</span>
      <input
        class="field-input"
        v-model="createInput"
        :placeholder="t('workspace.plugins.create.placeholder')"
        @keydown.enter="submitCreate"
      />
    </label>
    <p class="muted small dialog-row-help">
      {{ t('workspace.plugins.create.help') }}
    </p>
    <p v-if="createError" class="form-error">{{ createError }}</p>

    <template #footer>
      <button class="tool-btn" type="button" @click="createOpen = false">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="submitCreate">
        {{ t('workspace.plugins.create.submit') }}
      </button>
    </template>
  </Modal>

  <!-- Delete confirm -->
  <ConfirmDialog
    :open="deleteOpen"
    :title="t('workspace.plugins.delete.title')"
    :message="t('workspace.plugins.delete.confirm', [deleteName])"
    :confirm-label="t('workspace.profiles.action.delete')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="deleteOpen = false"
    @confirm="confirmDelete"
  />

  <!-- Import overwrite confirm — fires when the archive's plugin id
       collides with one already on disk. ConfirmDialog re-runs the
       import with overwrite=true; cancel discards the pending path. -->
  <ConfirmDialog
    :open="importOverwriteOpen"
    :title="t('workspace.plugins.archive.import.overwrite_title')"
    :message="t('workspace.plugins.archive.import.overwrite_confirm', [importPendingName])"
    :confirm-label="t('workspace.plugins.archive.import.overwrite_confirm_button')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="cancelImportOverwrite"
    @confirm="confirmImportOverwrite"
  />

  <!-- Form-editor: add/edit field. Plugins use a curated subset of
       field types — workflow-irrelevant types (image, list, table,
       link, api, guid, looper, tags) are hidden from the dropdown
       so plugin authors only see types that make sense for run-once
       input forms. -->
  <FieldEditModal
    :open="fieldEditOpen"
    :field="fieldEditTarget"
    :is-new="fieldEditIsNew"
    :allowed-types="[
      'text', 'textarea', 'number', 'boolean', 'dropdown',
      'multioption', 'radio', 'date', 'range',
      'file-path', 'folder-path',
    ]"
    @close="fieldEditOpen = false"
    @confirm="applyFieldEdit"
  />

  <!-- Form-editor: delete-field confirm -->
  <ConfirmDialog
    :open="fieldDeleteOpen"
    :title="t('workspace.plugins.form.delete_title')"
    :message="t('workspace.plugins.form.delete_confirm', [fieldDeleteName])"
    :confirm-label="t('workspace.profiles.action.delete')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="fieldDeleteOpen = false"
    @confirm="confirmDeleteField"
  />

  <!-- Run modal -->
  <Modal
    :open="runOpen && !!selectedPlugin"
    :title="
      runMode === 'form'
        ? (draftManifest?.name ?? selectedPlugin?.manifest.name ?? '')
        : t('workspace.plugins.run_title', [draftManifest?.name ?? selectedPlugin?.manifest.name ?? ''])
    "
    width="640px"
    @close="runOpen = false"
  >
    <div v-if="selectedPlugin" class="run-modal">
      <section
        v-if="runMode === 'form'"
        class="run-form"
      >
        <div
          v-if="descriptionHTML"
          class="section-info"
          v-html="descriptionHTML"
        ></div>
        <template v-for="(entry, i) in draftForm" :key="i">
          <ProgressBarWidget
            v-if="isWidget(entry) && entry.kind === WidgetKind.KindProgressBar"
            :widget="entry"
          />
          <StatusMessageWidget
            v-else-if="isWidget(entry) && entry.kind === WidgetKind.KindStatusMessage"
            :widget="entry"
          />
          <FormFieldRow
            v-else-if="!isWidget(entry)"
            :field="entry"
            :model-value="runValues[entry.key]"
            @update:model-value="(v: unknown) => (runValues[entry.key] = v)"
          />
        </template>
        <div
          v-if="visibleCommands.length > 0"
          class="run-form-buttons"
        >
          <button
            v-for="cmd in visibleCommands"
            :key="cmd.id"
            class="tool-btn primary"
            :disabled="runningCmd === cmd.id"
            @click="runCommand(selectedPlugin, cmd)"
          >
            <span v-if="runningCmd === cmd.id">
              {{ t('workspace.plugins.running') }}
            </span>
            <span v-else>{{ cmd.label || cmd.id }}</span>
          </button>
          <button
            v-if="globalRunning"
            class="tool-btn"
            type="button"
            :disabled="globalStopping"
            @click="stopGlobalRun"
          >
            {{ globalStopping ? t('workspace.plugins.stopping') : t('workspace.plugins.stop') }}
          </button>
        </div>

      </section>

      <template v-if="runMode !== 'form'">
      <section
        v-for="cmd in visibleCommands"
        :key="cmd.id"
        class="command-card"
      >
        <div class="command-header">
          <h3>{{ cmd.label || cmd.id }}</h3>
          <button
            class="tool-btn primary"
            :disabled="runningCmd === cmd.id"
            @click="runCommand(selectedPlugin, cmd)"
          >
            <span v-if="runningCmd === cmd.id">
              {{ t('workspace.plugins.running') }}
            </span>
            <span v-else>{{ t('workspace.plugins.run') }}</span>
          </button>
        </div>

      </section>
      </template>

      <PluginResultPanel
        :commands="visibleCommands"
        :results="runResults"
        :enabled="!!draftManifest?.debug"
      />
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="runOpen = false">
        {{ t('common.close') }}
      </button>
    </template>
  </Modal>
</template>
