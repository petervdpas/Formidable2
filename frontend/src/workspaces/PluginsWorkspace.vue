<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import CodeEditor from "../components/CodeEditor.vue";
import {
  Service as PluginSvc,
  Command,
  RunResultDTO,
  type ListResult,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import {
  FormSection,
  FormRow,
  TextField,
  TextareaField,
} from "../components/fields";
import { useRestartGate } from "../composables/useRestartGate";
import { useToast } from "../composables/useToast";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { usePlugins, isValidPluginID } from "../composables/usePlugins";
import { usePluginEditor } from "../composables/usePluginEditor";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const toast = useToast();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const {
  plugins,
  selectedID,
  selectedPlugin,
  refresh,
  create: createPlugin,
  remove,
} = usePlugins();

const { draftManifest, draftSource, dirty, save, reset } = usePluginEditor();

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

// ── Run dialog ───────────────────────────────────────────────────────
const runOpen = ref(false);
// Latest run result keyed by command id so re-opening the modal
// shows the previous output until the user runs again.
const runResults = ref<Record<string, RunResultDTO>>({});
const runningCmd = ref<string>("");

function openRun() {
  if (!selectedPlugin.value) return;
  runOpen.value = true;
}

async function runCommand(p: ListResult, cmd: Command) {
  runningCmd.value = cmd.id;
  try {
    runResults.value[cmd.id] = await PluginSvc.Run(p.id, cmd.id, {});
  } catch (err) {
    runResults.value[cmd.id] = new RunResultDTO({
      kind: "runtime_error",
      message: String(err),
    });
  } finally {
    runningCmd.value = "";
  }
}

function prettyValue(v: unknown): string {
  if (v === undefined || v === null) {
    return t("workspace.plugins.no_output");
  }
  if (typeof v === "string") return v;
  return JSON.stringify(v, null, 2);
}

function errorLabel(kind: string, message: string): string {
  if (kind === "plugin_not_found") {
    return t("workspace.plugins.error_plugin_not_found", [message]);
  }
  if (kind === "command_not_found") {
    return t("workspace.plugins.error_command_not_found", [message]);
  }
  return t("workspace.plugins.error_runtime");
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
setTopbarMenu(() => [
  {
    type: "group",
    id: "file",
    labelKey: "menu.file",
    items: [
      {
        id: "save",
        labelKey: "workspace.plugins.save",
        disabled: !dirty.value,
        onClick: doSave,
      },
      {
        id: "reset",
        labelKey: "workspace.plugins.reset",
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
      { type: "separator", id: "sep-run" },
      {
        id: "run",
        labelKey: "menu.plugin.run",
        // Run is meaningful only when a plugin is selected and has
        // at least one command on it.
        disabled:
          !selectedPlugin.value ||
          (selectedPlugin.value.manifest.commands?.length ?? 0) === 0,
        onClick: openRun,
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
        {{ t('workspace.plugins.dirty_indicator') }}
      </span>
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
            <span class="badge small">{{ p.id }}</span>
            <span class="muted small">v{{ p.manifest.version }}</span>
          </span>
        </li>
      </ul>
    </template>

    <template #main>
      <p v-if="!selectedPlugin || !draftManifest" class="workspace-empty">
        {{ t('workspace.plugins.empty_main') }}
      </p>

      <template v-else>
        <div class="workspace-heading-row">
          <h1 class="workspace-heading">{{ draftManifest.name || selectedID }}</h1>
          <span class="badge badge-accent">{{ selectedID }}</span>
          <span v-if="dirty" class="badge badge-warn">
            {{ t('workspace.plugins.dirty_indicator') }}
          </span>
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

        <FormSection :title="t('workspace.plugins.commands.title')">
          <p v-if="!draftManifest.commands || draftManifest.commands.length === 0" class="muted small">
            {{ t('workspace.plugins.commands.empty') }}
          </p>
          <ul v-else class="cmd-rows">
            <li
              v-for="(c, i) in draftManifest.commands"
              :key="i"
              class="cmd-row"
            >
              <div class="cmd-row-grid">
                <label class="cmd-row-label">
                  <span class="muted small">{{ t('workspace.plugins.commands.id') }}</span>
                  <input class="field-input" v-model="c.id" />
                </label>
                <label class="cmd-row-label">
                  <span class="muted small">{{ t('workspace.plugins.commands.label') }}</span>
                  <input class="field-input" v-model="c.label" />
                </label>
                <label class="cmd-row-label">
                  <span class="muted small">{{ t('workspace.plugins.commands.fn') }}</span>
                  <input
                    class="field-input"
                    v-model="c.fn"
                    :placeholder="c.id || t('workspace.plugins.commands.fn_placeholder')"
                  />
                </label>
                <button
                  type="button"
                  class="field-action-btn delete"
                  @click="removeCommand(i)"
                >
                  {{ t('workspace.plugins.commands.delete') }}
                </button>
              </div>
            </li>
          </ul>
          <div class="cmd-add-row">
            <button class="tool-btn" type="button" @click="addCommand">
              + {{ t('workspace.plugins.commands.add') }}
            </button>
          </div>
        </FormSection>

        <FormSection :title="t('workspace.plugins.source.title')">
          <div class="plugin-source">
            <CodeEditor v-model="draftSource" lang="lua" :height="360" />
            <p class="muted small">{{ t('workspace.plugins.source.help') }}</p>
          </div>
        </FormSection>
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

  <!-- Run modal -->
  <Modal
    :open="runOpen && !!selectedPlugin"
    :title="selectedPlugin ? t('workspace.plugins.run_title', [selectedPlugin.manifest.name]) : ''"
    width="640px"
    @close="runOpen = false"
  >
    <div v-if="selectedPlugin" class="run-modal">
      <section
        v-for="cmd in selectedPlugin.manifest.commands"
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

        <div v-if="runResults[cmd.id]" class="command-result">
          <template v-if="runResults[cmd.id]!.kind === 'ok'">
            <h4>{{ t('workspace.plugins.output_title') }}</h4>
            <pre class="result-output">{{ prettyValue(runResults[cmd.id]!.value) }}</pre>
          </template>
          <template v-else>
            <h4 class="error-heading">
              {{
                errorLabel(
                  runResults[cmd.id]!.kind,
                  runResults[cmd.id]!.message ?? '',
                )
              }}
            </h4>
            <pre class="result-output error-output">{{ runResults[cmd.id]!.message }}</pre>
          </template>

          <template v-if="(runResults[cmd.id]!.logLines?.length ?? 0) > 0">
            <h4>{{ t('workspace.plugins.logs_title') }}</h4>
            <pre class="result-logs">{{ runResults[cmd.id]!.logLines!.join('\n') }}</pre>
          </template>
        </div>
      </section>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="runOpen = false">
        {{ t('common.close') }}
      </button>
    </template>
  </Modal>
</template>
