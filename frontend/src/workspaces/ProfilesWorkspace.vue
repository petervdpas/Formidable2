<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import AlertDialog from "../components/AlertDialog.vue";
import { useProfiles, isValidProfileFilename } from "../composables/useProfiles";
import { useConfig } from "../composables/useConfig";
import { useRestartGate } from "../composables/useRestartGate";
import { useActiveWorkspace } from "../composables/useActiveWorkspace";
import { useDialog } from "../composables/useDialog";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useWorkspacePluginMenu } from "../composables/useWorkspacePluginMenu";
import { useToast } from "../composables/useToast";
import { useRestartFlow } from "../composables/useRestartFlow";

const { t } = useI18n();

const { profiles, activeFilename, refresh, activate, create, remove, exportTo, importFrom } = useProfiles();
const { config } = useConfig();

// Explicit key map (no interpolation) for the active profile's backend label.
const BACKEND_LABEL_KEYS: Record<string, string> = {
  none: "backend.none",
  git: "backend.git",
  gigot: "backend.gigot",
};
const remoteBackendLabel = computed(() =>
  t(BACKEND_LABEL_KEYS[config.value?.remote_backend ?? "none"] ?? "backend.none"),
);

const { bootConfig } = useRestartGate();
const { setActive } = useActiveWorkspace();
const { chooseFile, chooseSaveFile } = useDialog();
const toast = useToast();

async function doRefresh() {
  try {
    await refresh();
    toast.success("toast.refresh.success");
  } catch (err) {
    toast.error("toast.refresh.error", [String(err)]);
  }
}

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const jsonFilters = computed(() => [
  { displayName: t('workspace.profiles.import.filter_name'), pattern: '*.json' },
]);

// Sidebar selection - separate from "active" so the user can browse
// without flipping the live profile.
const selectedFilename = ref<string>(activeFilename.value);

const selectedEntry = computed(
  () => profiles.value.find((p) => p.value === selectedFilename.value),
);
const isActiveSelected = computed(
  () => selectedFilename.value !== "" && selectedFilename.value === activeFilename.value,
);

// ── Create modal state ───────────────────────────────────────────────
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
  if (!isValidProfileFilename(name)) {
    createError.value = t("workspace.profiles.create.invalid");
    return;
  }
  const result = await create(name);
  if (!result.ok) {
    createError.value =
      result.code === "exists"
        ? t("workspace.profiles.create.exists")
        : t("workspace.profiles.create.error", [result.message ?? "?"]);
    return;
  }
  // Successfully created + activated.
  selectedFilename.value = name;
  createOpen.value = false;
}

// ── Delete confirm state ─────────────────────────────────────────────
const deleteOpen = ref(false);
const deleteError = ref<string>("");

function openDelete() {
  deleteError.value = "";
  deleteOpen.value = true;
}

async function submitDelete() {
  const target = selectedFilename.value;
  if (!target) {
    deleteOpen.value = false;
    return;
  }
  const result = await remove(target);
  if (!result?.success) {
    switch (result?.code) {
      case "active_profile":
        deleteError.value = t("workspace.profiles.delete.error_active");
        break;
      case "boot_forbidden":
        deleteError.value = t("workspace.profiles.delete.error_boot");
        break;
      default:
        deleteError.value = t("workspace.profiles.delete.error_generic", [
          result?.error ?? "?",
        ]);
    }
    return;
  }
  // Deleted - clear selection and close.
  if (selectedFilename.value === target) {
    selectedFilename.value = activeFilename.value;
  }
  deleteOpen.value = false;
}

// ── Alert dialog (single-OK feedback) ────────────────────────────────
const alertOpen = ref(false);
const alertTitle = ref<string>("");
const alertMessage = ref<string>("");
const alertVariant = ref<"default" | "danger">("default");

function showAlert(title: string, message: string, variant: "default" | "danger" = "default") {
  alertTitle.value = title;
  alertMessage.value = message;
  alertVariant.value = variant;
  alertOpen.value = true;
}

// ── Import flow ──────────────────────────────────────────────────────
const overwriteOpen = ref(false);
const overwriteFilename = ref<string>("");
const overwriteSourcePath = ref<string>("");

async function importProfile() {
  const path = await chooseFile(jsonFilters.value);
  if (!path) return; // user cancelled
  await runImport(path, false);
}

async function runImport(sourcePath: string, overwrite: boolean) {
  const result = await importFrom(sourcePath, overwrite);
  if (result?.success) {
    if (result.filename) selectedFilename.value = result.filename;
    showAlert(
      t('common.alert_title'),
      t('workspace.profiles.import.success', [result.filename ?? '?']),
    );
    return;
  }
  switch (result?.code) {
    case 'exists':
      overwriteFilename.value = result.filename ?? '?';
      overwriteSourcePath.value = sourcePath;
      overwriteOpen.value = true;
      return;
    case 'not_found':
      showAlert(t('common.error_title'), t('workspace.profiles.import.error_not_found'), 'danger');
      return;
    case 'invalid_name':
      showAlert(t('common.error_title'), t('workspace.profiles.import.error_invalid_name'), 'danger');
      return;
    case 'boot_forbidden':
      showAlert(t('common.error_title'), t('workspace.profiles.import.error_boot_forbidden'), 'danger');
      return;
    case 'invalid_config':
      showAlert(t('common.error_title'), t('workspace.profiles.import.error_invalid_config'), 'danger');
      return;
    case 'copy_failed':
      showAlert(
        t('common.error_title'),
        t('workspace.profiles.import.error_copy_failed', [result?.error ?? '?']),
        'danger',
      );
      return;
    default:
      showAlert(
        t('common.error_title'),
        t('workspace.profiles.import.error_generic', [result?.error ?? '?']),
        'danger',
      );
  }
}

async function confirmOverwrite() {
  overwriteOpen.value = false;
  await runImport(overwriteSourcePath.value, true);
}

// ── Export flow ──────────────────────────────────────────────────────
async function exportProfile() {
  const target = selectedFilename.value;
  if (!target) return;
  const path = await chooseSaveFile(target, jsonFilters.value);
  if (!path) return;
  // Save dialog already obtained user consent for overwrite, so pass true.
  const result = await exportTo(target, path, true);
  if (result?.success) {
    showAlert(t('common.alert_title'), t('workspace.profiles.export.success', [path]));
    return;
  }
  switch (result?.code) {
    case 'not_found':
      showAlert(t('common.error_title'), t('workspace.profiles.export.error_not_found'), 'danger');
      return;
    case 'copy_failed':
      showAlert(
        t('common.error_title'),
        t('workspace.profiles.export.error_copy_failed', [result?.error ?? '?']),
        'danger',
      );
      return;
    default:
      showAlert(
        t('common.error_title'),
        t('workspace.profiles.export.error_generic', [result?.error ?? '?']),
        'danger',
      );
  }
}

// ── Activate / Edit-in-Settings ──────────────────────────────────────
// Switching profiles flips .boot.json + reloads the active config from
// disk, but workspace caches (templates list, storage selection, VFS,
// etc.) were keyed off the old profile and don't refresh in place - a
// live-switch leaves the UI showing stale rows from the previous repo.
// Until those caches grow profile-aware invalidation, the safe path is
// a real process restart: the boot pointer already points at the new
// profile, so the relaunched app comes up clean on the new context.
//
// pendingActivate is local because the dialog message embeds the
// filename; the restart machinery itself lives in useRestartFlow.
const restart = useRestartFlow();
const pendingActivate = ref<string | null>(null);

function requestActivate(filename: string) {
  pendingActivate.value = filename;
  restart.request({
    before: () => activate(filename),
    errorKey: "workspace.profiles.activate.error",
  });
}

function activateSelected() {
  if (!selectedFilename.value) return;
  requestActivate(selectedFilename.value);
}

function editInSettings() {
  // Already-active profile: pure navigation, no restart.
  if (isActiveSelected.value) {
    setActive("settings");
    return;
  }
  // Otherwise treat it like Activate - the user lands on the default
  // workspace after restart and clicks Settings themselves.
  requestActivate(selectedFilename.value);
}

function cancelActivate() {
  pendingActivate.value = null;
  restart.cancel();
}

// Topbar menu - File group with Import (always enabled) and Export
// (disabled when no profile is selected). The Apply auto-disable rule
// keeps the File button itself enabled as long as Import works.
const { buildMenu: buildPluginsMenu } = useWorkspacePluginMenu("profiles");
setTopbarMenu(() => [
  {
    type: "group",
    id: "file",
    labelKey: "menu.file",
    items: [
      {
        id: "import",
        labelKey: "workspace.profiles.import",
        onClick: importProfile,
      },
      {
        id: "export",
        labelKey: "workspace.profiles.action.export",
        disabled: !selectedEntry.value,
        onClick: exportProfile,
      },
      { type: "separator", id: "sep-refresh" },
      {
        id: "refresh",
        labelKey: "common.refresh",
        onClick: doRefresh,
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
      <button class="tool-btn primary" @click="openCreate">
        + {{ t('workspace.profiles.new_profile') }}
      </button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.profiles.sidebar_title') }}</h2>

      <p v-if="profiles.length === 0" class="muted small">
        {{ t('workspace.profiles.empty') }}
      </p>

      <ul v-else class="profile-list">
        <li
          v-for="p in profiles"
          :key="p.value"
          :class="[
            'sidebar-row',
            'sidebar-row--stack',
            { active: p.value === selectedFilename },
          ]"
          @click="selectedFilename = p.value"
        >
          <span class="profile-display">{{ p.display }}</span>
          <span class="profile-meta">
            <Badge
              v-if="p.value === activeFilename"
              variant="ok"
              class="small"
            >{{ t('workspace.profiles.active') }}</Badge>
            <Badge class="small profile-filename">{{ p.value }}</Badge>
          </span>
        </li>
      </ul>
    </template>

    <template #main>
      <p
        v-if="!selectedEntry"
        class="workspace-empty"
      >{{ t('workspace.profiles.unselected') }}</p>

      <template v-else>
        <h1 class="workspace-heading">
          {{ t('workspace.profiles.detail.title', [selectedEntry.display]) }}
        </h1>

        <div class="profile-detail-meta">
          <Badge variant="accent">{{ selectedEntry.value }}</Badge>
          <Badge
            v-if="isActiveSelected"
            variant="ok"
          >{{ t('workspace.profiles.active') }}</Badge>
        </div>

        <!-- Properties only fully readable when this profile is the
             active one (others aren't loaded into reactive state). -->
        <dl v-if="isActiveSelected && config" class="kv profile-kv">
          <dt>{{ t('workspace.profiles.detail.profile_name') }}</dt>
          <dd>{{ config.profile_name || '-' }}</dd>
          <dt>{{ t('workspace.profiles.detail.author_name') }}</dt>
          <dd>{{ config.author_name || '-' }}</dd>
          <dt>{{ t('workspace.profiles.detail.author_email') }}</dt>
          <dd>{{ config.author_email || '-' }}</dd>
          <dt>{{ t('workspace.profiles.detail.theme') }}</dt>
          <dd>{{ t('theme.' + (config.theme || 'light')) }}</dd>
          <dt>{{ t('workspace.profiles.detail.language') }}</dt>
          <dd>{{ config.language || '-' }}</dd>
          <dt>{{ t('workspace.profiles.detail.context_folder') }}</dt>
          <dd><code>{{ config.context_folder || '-' }}</code></dd>
          <dt>{{ t('settings.field.remote_backend') }}</dt>
          <dd>{{ remoteBackendLabel }}</dd>
        </dl>
        <p v-else class="muted">
          {{ t('workspace.profiles.detail.unloaded') }}
        </p>

        <div class="profile-actions">
          <button class="tool-btn primary" @click="editInSettings">
            {{ t('workspace.profiles.action.edit_in_settings') }}
          </button>
          <button
            class="tool-btn"
            :disabled="isActiveSelected"
            @click="activateSelected"
          >
            {{ t('workspace.profiles.action.activate') }}
          </button>
          <button
            class="tool-btn danger"
            :disabled="isActiveSelected"
            @click="openDelete"
          >
            {{ t('workspace.profiles.action.delete') }}
          </button>
        </div>
      </template>
    </template>
  </SplitPane>

  <!-- Create modal ----------------------------------------------------->
  <Modal
    :open="createOpen"
    :title="t('workspace.profiles.create.title')"
    @close="createOpen = false"
  >
    <label class="dialog-row">
      <span class="dialog-row-label">{{ t('workspace.profiles.create.label') }}</span>
      <input
        class="field-input"
        v-model="createInput"
        :placeholder="t('workspace.profiles.create.placeholder')"
        @keydown.enter="submitCreate"
      />
    </label>
    <p class="muted small dialog-row-help">
      {{ t('workspace.profiles.create.help') }}
    </p>
    <p v-if="createError" class="form-error">{{ createError }}</p>

    <template #footer>
      <button class="tool-btn" type="button" @click="createOpen = false">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="submitCreate">
        {{ t('workspace.profiles.new_profile') }}
      </button>
    </template>
  </Modal>

  <!-- Delete confirm --------------------------------------------------->
  <ConfirmDialog
    :open="deleteOpen"
    :title="t('workspace.profiles.delete.title')"
    :confirm-label="t('workspace.profiles.action.delete')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="deleteOpen = false"
    @confirm="submitDelete"
  >
    <p class="confirm-message">
      {{ t('workspace.profiles.delete.confirm', [selectedEntry?.value ?? '']) }}
    </p>
    <p v-if="deleteError" class="form-error">{{ deleteError }}</p>
  </ConfirmDialog>

  <!-- Import: overwrite confirm --------------------------------------->
  <ConfirmDialog
    :open="overwriteOpen"
    :title="t('workspace.profiles.import.overwrite_title')"
    :confirm-label="t('workspace.profiles.import.overwrite_button')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="overwriteOpen = false"
    @confirm="confirmOverwrite"
  >
    <p class="confirm-message">
      {{ t('workspace.profiles.import.overwrite_confirm', [overwriteFilename]) }}
    </p>
  </ConfirmDialog>

  <!-- Generic alert (success / error feedback) ------------------------->
  <AlertDialog
    :open="alertOpen"
    :title="alertTitle"
    :message="alertMessage"
    :variant="alertVariant"
    @close="alertOpen = false"
  />

  <!-- Activate (= switch profile + restart) confirm ------------------->
  <ConfirmDialog
    :open="restart.confirmOpen.value"
    :title="t('workspace.profiles.activate.confirm.title')"
    :message="t('workspace.profiles.activate.confirm.body', [pendingActivate ?? ''])"
    :confirm-label="t('workspace.profiles.activate.confirm.button')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="cancelActivate"
    @confirm="restart.confirm"
  />

  <AlertDialog
    :open="restart.errorOpen.value"
    :title="t('common.error_title')"
    :message="restart.errorMessage.value"
    variant="danger"
    @close="restart.dismissError"
  />
</template>

