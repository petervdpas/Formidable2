<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import ConfirmDialog from "../../components/ConfirmDialog.vue";
import ProgressBar from "../../components/ProgressBar.vue";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import type { SyncProgress } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";
import { useConfig } from "../../composables/useConfig";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// Repository Sync — gigot edition. Initial scope: Clone (fetch the
// server's HEAD into the configured context folder) + Re-clone
// (wipe managed paths first, then fetch). Both target the active
// profile's gigot_base_url / gigot_repo_name; the subscription bearer
// is resolved server-side from the OS keychain so no secret crosses
// the Wails bridge.
//
// Push / Pull / Sync land here in the next iteration once the
// commit / journal-status surface is fleshed out.

const { t } = useI18n();
const { config } = useConfig();
const toast = useToast();

const contextFolder = computed(() => config.value?.context_folder ?? "");
const baseURL = computed(() => config.value?.gigot_base_url ?? "");
const repoName = computed(() => config.value?.gigot_repo_name ?? "");

const cloning = ref(false);
const recloning = ref(false);
const confirmReclone = ref(false);

const progressCurrent = ref(0);
const progressTotal = ref(0);
const progressPath = ref("");

const inFlight = computed(() => cloning.value || recloning.value);
const configured = computed(
  () =>
    contextFolder.value.trim() !== ""
    && baseURL.value.trim() !== ""
    && repoName.value.trim() !== "",
);

const canAct = computed(() => !inFlight.value && configured.value);

const progressLabel = computed(() => {
  const op = cloning.value
    ? t("workspace.collaboration.gigot.sync.clone.running")
    : recloning.value
      ? t("workspace.collaboration.gigot.sync.reclone.running")
      : "";
  if (!op) return "";
  if (progressPath.value) {
    return `${op} ${progressPath.value}`;
  }
  return op;
});

function resetProgress() {
  progressCurrent.value = 0;
  progressTotal.value = 0;
  progressPath.value = "";
}

let unsubscribeProgress: (() => void) | null = null;

onMounted(() => {
  unsubscribeProgress = Events.On(
    "gigot:sync_progress",
    (ev: { data?: SyncProgress } | SyncProgress) => {
      const data = (ev as { data?: SyncProgress })?.data ?? (ev as SyncProgress);
      if (!data || typeof data !== "object") return;
      progressCurrent.value = data.Current ?? 0;
      progressTotal.value = data.Total ?? 0;
      progressPath.value = data.Path ?? "";
    },
  );
});

onBeforeUnmount(() => {
  unsubscribeProgress?.();
  unsubscribeProgress = null;
});

async function doClone() {
  if (!canAct.value) return;
  resetProgress();
  cloning.value = true;
  try {
    const res = await GigotSvc.PullLocal();
    if (!res) throw new Error("no response");
    toast.success("workspace.collaboration.gigot.sync.clone.success", [
      String(res.files ?? 0),
      String(res.deleted ?? 0),
      res.version ?? "",
    ]);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.sync.clone.error", [backendErrMessage(err)]);
  } finally {
    cloning.value = false;
    resetProgress();
  }
}

function askReclone() {
  if (!canAct.value) return;
  confirmReclone.value = true;
}

async function doReclone() {
  confirmReclone.value = false;
  if (!canAct.value) return;
  resetProgress();
  recloning.value = true;
  try {
    const res = await GigotSvc.Reclone();
    if (!res) throw new Error("no response");
    toast.success("workspace.collaboration.gigot.sync.reclone.success", [
      String(res.files ?? 0),
      res.version ?? "",
    ]);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.sync.reclone.error", [backendErrMessage(err)]);
  } finally {
    recloning.value = false;
    resetProgress();
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.gigot.sync.info') }}</p>

  <div v-if="!configured && !inFlight" class="gigot-sync-note">
    {{ t('workspace.collaboration.gigot.sync.not_configured') }}
  </div>

  <ProgressBar
    :active="inFlight"
    :label="progressLabel"
    :current="progressCurrent"
    :total="progressTotal"
  />

  <div class="gigot-sync-actions">
    <button
      type="button"
      class="tool-btn primary"
      :disabled="!canAct"
      @click="doClone"
    >
      {{ cloning ? t('workspace.collaboration.gigot.sync.clone.running') : t('workspace.collaboration.gigot.sync.clone.button') }}
    </button>
    <button
      type="button"
      class="tool-btn danger"
      :disabled="!canAct"
      @click="askReclone"
    >
      {{ recloning ? t('workspace.collaboration.gigot.sync.reclone.running') : t('workspace.collaboration.gigot.sync.reclone.button') }}
    </button>
  </div>

  <ConfirmDialog
    :open="confirmReclone"
    :title="t('workspace.collaboration.gigot.sync.reclone.confirm_title')"
    :message="t('workspace.collaboration.gigot.sync.reclone.confirm_message', [contextFolder])"
    :confirm-label="t('workspace.collaboration.gigot.sync.reclone.button')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="confirmReclone = false"
    @confirm="doReclone"
  />
</template>
