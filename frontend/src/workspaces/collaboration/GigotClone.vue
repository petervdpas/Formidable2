<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import ConfirmDialog from "../../components/ConfirmDialog.vue";
import ProgressBar from "../../components/ProgressBar.vue";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
} from "../../components/fields";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import { Service as CredentialSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/credential";
import type { SyncProgress } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";
import { useConfig } from "../../composables/useConfig";
import { useCredentialAccount } from "../../composables/useCredentialAccount";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// Clone Repository — gigot edition. Folds setup (subscription bearer
// entry + keychain save toggle) into the same panel as Clone /
// Re-clone, so gigot has a single setup-and-clone section that
// mirrors git's Clone Repository. Clone is a merge-aware pull (server
// deletes apply locally, local-only files survive); Re-clone wipes
// managed paths first. Push / Pull / Sync — the day-to-day ops — live
// in Repository Sync.
//
// Token handling mirrors git's PAT pattern: the field is transient
// and always cleared after the action. Leave blank to use the
// previously saved bearer from the OS keychain. If filled, the value
// is written to the keychain so the backend op can resolve it; when
// the save toggle is OFF the entry is deleted again afterwards so
// the secret doesn't persist across sessions.

const { t } = useI18n();
const { config } = useConfig();
const { accountFor } = useCredentialAccount();
const toast = useToast();

const contextFolder = computed(() => config.value?.context_folder ?? "");
const baseURL = computed(() => config.value?.gigot_base_url ?? "");
const repoName = computed(() => config.value?.gigot_repo_name ?? "");

const token = ref("");
const saveToken = ref(true);

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

// Token-handling around a single backend op. Returns a cleanup fn
// the caller must invoke in its `finally` — guarantees we never
// leak a transient bearer into the keychain on the save=off path.
async function withTransientToken(): Promise<() => Promise<void>> {
  const provided = token.value;
  if (provided === "") {
    return async () => {};
  }
  const account = accountFor("gigot", repoName.value.trim());
  await CredentialSvc.Set(account, provided);
  const persist = saveToken.value;
  return async () => {
    if (!persist) {
      try {
        await CredentialSvc.Delete(account);
      } catch {
        /* keychain delete failure is non-fatal — same rationale as
           the prior GigotConnect flow */
      }
    }
  };
}

async function doClone() {
  if (!canAct.value) return;
  resetProgress();
  cloning.value = true;
  let cleanup: () => Promise<void> = async () => {};
  try {
    cleanup = await withTransientToken();
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
    await cleanup();
    token.value = "";
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
  let cleanup: () => Promise<void> = async () => {};
  try {
    cleanup = await withTransientToken();
    const res = await GigotSvc.Reclone();
    if (!res) throw new Error("no response");
    toast.success("workspace.collaboration.gigot.clone.reclone.success", [
      String(res.files ?? 0),
      res.version ?? "",
    ]);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.clone.reclone.error", [backendErrMessage(err)]);
  } finally {
    await cleanup();
    token.value = "";
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

  <FormSection>
    <FormRow
      :label="t('workspace.collaboration.gigot.clone.token')"
      :description="t('workspace.collaboration.gigot.clone.token_help')"
    >
      <TextField v-model="token" type="password" autocomplete="off" />
    </FormRow>
    <FormSwitchRow
      :label="t('workspace.collaboration.gigot.clone.save_token')"
      :description="t('workspace.collaboration.gigot.clone.save_token_help')"
      v-model="saveToken"
    />
  </FormSection>

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
