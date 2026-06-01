<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import { FormSection, FormRow, TextField, FolderPathField } from "../fields";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import { useRemoteConfig } from "../../composables/useRemoteConfig";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// Inline form on Current Service. Mirrors GitConnection's role:
// non-secret addressing fields only. The subscription bearer lives
// in the OS keychain (account "<profile>:gigot:<repoName>") and is
// captured by the "Connect to GiGot" workspace section, not here -
// plaintext secrets do not belong in the profile JSON.
//
// Quick-action row: when the connection is wired up (config complete
// AND Me() probe succeeds), surface a one-click Sync button. Pending
// local changes hard-disable the button so the user can't bypass the
// commit-message requirement - Sync() from here would otherwise fall
// through to the auto-generated audit string, which is explicitly the
// wrong UX. The probe also listens for journal:changed so an edit on
// another page re-disables Sync without the user revisiting this one.

const { t } = useI18n();
const { config, update, gigotBaseURL: baseURL, gigotRepoName: repoName, contextFolder } = useRemoteConfig();
const toast = useToast();
const cfg = computed(() => config.value!);

const configured = computed(
  () => contextFolder.value.trim() !== ""
    && baseURL.value.trim() !== ""
    && repoName.value.trim() !== "",
);

const probing = ref(false);
const isConnected = ref(false);
const hasPending = ref(false);
const syncing = ref(false);

async function probeConnection() {
  if (!configured.value) {
    isConnected.value = false;
    return;
  }
  probing.value = true;
  try {
    await GigotSvc.Me();
    isConnected.value = true;
  } catch {
    isConnected.value = false;
  } finally {
    probing.value = false;
  }
}

async function refreshPendingState() {
  if (!configured.value) {
    hasPending.value = false;
    return;
  }
  try {
    const s = await GigotSvc.LedgerSummary();
    hasPending.value = (s?.changed?.length ?? 0) > 0
      || (s?.deleted?.length ?? 0) > 0;
  } catch {
    // Read-only diff - keep last-known value rather than flipping
    // to "clean" on a transient error. A subsequent journal:changed
    // event will re-probe.
  }
}

let unsubscribeJournal: (() => void) | null = null;

onMounted(() => {
  void probeConnection();
  void refreshPendingState();
  unsubscribeJournal = Events.On("journal:changed", () => {
    void refreshPendingState();
  });
});

onBeforeUnmount(() => {
  unsubscribeJournal?.();
  unsubscribeJournal = null;
});

watch(
  () => [baseURL.value, repoName.value, contextFolder.value] as const,
  () => {
    void probeConnection();
    void refreshPendingState();
  },
);

const canQuickSync = computed(
  () => isConnected.value && !syncing.value && !hasPending.value,
);
const quickSyncDisabledHint = computed(() => {
  if (!isConnected.value) return "";
  if (hasPending.value) return t("workspace.collaboration.gigot.quicksync.pending_block");
  return "";
});

async function doQuickSync() {
  if (!canQuickSync.value) return;
  syncing.value = true;
  try {
    // Belt-and-suspenders: hasPending may be stale if the user saved
    // a record between the last probe and this click. Re-check before
    // we let an auto-message commit slip through.
    const summary = await GigotSvc.LedgerSummary();
    const pending = (summary?.changed?.length ?? 0) > 0
      || (summary?.deleted?.length ?? 0) > 0;
    if (pending) {
      hasPending.value = true;
      toast.warn("workspace.collaboration.gigot.quicksync.pending_block");
      return;
    }
    const res = await GigotSvc.Sync("");
    if (!res) return;
    if (res.noop) {
      toast.info("workspace.collaboration.gigot.quicksync.noop");
    } else {
      const v = (res.version ?? "").slice(0, 8);
      toast.success("workspace.collaboration.gigot.quicksync.success", [v]);
    }
    await refreshPendingState();
  } catch (err) {
    toast.error("workspace.collaboration.gigot.quicksync.error", [backendErrMessage(err)]);
  } finally {
    syncing.value = false;
  }
}
</script>

<template>
  <FormSection v-if="cfg">
    <FormRow :label="t('settings.field.context_directory')">
      <FolderPathField
        :model-value="cfg.context_folder"
        @update:model-value="(v) => update({ context_folder: v })"
        placeholder="./Examples"
      />
    </FormRow>
    <FormRow :label="t('settings.field.gigot_base_url')">
      <TextField
        :model-value="cfg.gigot?.base_url"
        @update:model-value="(v) => update({ gigot: { base_url: v } })"
        placeholder="https://gigot.example.com"
      />
    </FormRow>
    <FormRow :label="t('settings.field.gigot_repository')">
      <TextField
        :model-value="cfg.gigot?.repo_name"
        @update:model-value="(v) => update({ gigot: { repo_name: v } })"
      />
    </FormRow>
  </FormSection>

  <div
    v-if="isConnected && !probing"
    class="gigot-quicksync-row"
  >
    <span class="gigot-quicksync-status">
      {{ t('workspace.collaboration.gigot.quicksync.connected') }}
    </span>
    <button
      type="button"
      class="tool-btn primary"
      :disabled="!canQuickSync"
      :title="quickSyncDisabledHint"
      @click="doQuickSync"
    >
      {{ syncing
        ? t('workspace.collaboration.gigot.quicksync.running')
        : t('workspace.collaboration.gigot.quicksync.button') }}
    </button>
  </div>
</template>
