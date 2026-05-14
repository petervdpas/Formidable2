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

// Clone Repository — gigot edition. Mirrors git's Clone Repository
// section: ingests the configured server's HEAD into the active
// context folder. Clone is a merge-aware pull; Re-clone wipes managed
// paths first. Push / Pull / Fetch — the actual sync ops — live in the
// future Repository Sync section, parallel to git/Sync.vue.

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
    ? t("workspace.collaboration.gigot.clone.running")
    : recloning.value
      ? t("workspace.collaboration.gigot.clone.reclone.running")
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
      progressCurrent.value = data.current ?? 0;
      progressTotal.value = data.total ?? 0;
      progressPath.value = data.path ?? "";
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
    toast.success("workspace.collaboration.gigot.clone.success", [
      String(res.files ?? 0),
      String(res.deleted ?? 0),
      res.version ?? "",
    ]);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.clone.error", [backendErrMessage(err)]);
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
    toast.success("workspace.collaboration.gigot.clone.reclone.success", [
      String(res.files ?? 0),
      res.version ?? "",
    ]);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.clone.reclone.error", [backendErrMessage(err)]);
  } finally {
    recloning.value = false;
    resetProgress();
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.gigot.clone.info') }}</p>

  <div v-if="!configured && !inFlight" class="gigot-clone-note">
    {{ t('workspace.collaboration.gigot.clone.not_configured') }}
  </div>

  <ProgressBar
    :active="inFlight"
    :label="progressLabel"
    :current="progressCurrent"
    :total="progressTotal"
  />

  <div class="gigot-clone-actions">
    <button
      type="button"
      class="tool-btn primary"
      :disabled="!canAct"
      @click="doClone"
    >
      {{ cloning ? t('workspace.collaboration.gigot.clone.running') : t('workspace.collaboration.gigot.clone.button') }}
    </button>
    <button
      type="button"
      class="tool-btn danger"
      :disabled="!canAct"
      @click="askReclone"
    >
      {{ recloning ? t('workspace.collaboration.gigot.clone.reclone.running') : t('workspace.collaboration.gigot.clone.reclone.button') }}
    </button>
  </div>

  <ConfirmDialog
    :open="confirmReclone"
    :title="t('workspace.collaboration.gigot.clone.reclone.confirm_title')"
    :message="t('workspace.collaboration.gigot.clone.reclone.confirm_message', [contextFolder])"
    :confirm-label="t('workspace.collaboration.gigot.clone.reclone.button')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="confirmReclone = false"
    @confirm="doReclone"
  />
</template>
