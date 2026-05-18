<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import Badge from "../../components/Badge.vue";
import ConfirmDialog from "../../components/ConfirmDialog.vue";
import ProgressBar from "../../components/ProgressBar.vue";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
  FolderPathField,
} from "../../components/fields";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import { Service as CredentialSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/credential";
import type { SyncProgress } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";
import { useConfig } from "../../composables/useConfig";
import { useCredentialAccount } from "../../composables/useCredentialAccount";
import { useCredentialStatus } from "../../composables/useCredentialStatus";
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
const { config, profileFilename, update } = useConfig();
const { accountFor } = useCredentialAccount();
const toast = useToast();

// Addressing fields edit the active profile through useConfig so a
// Clone here updates Current Service + Repository Sync in lockstep.
// (The same fields surface on Current Service via GigotConnection —
// that's intentional. Either entry point fills the same config.)
const contextFolder = computed({
  get: () => config.value?.context_folder ?? "",
  set: (v: string) => void update({ context_folder: v }),
});
const baseURL = computed({
  get: () => config.value?.gigot_base_url ?? "",
  set: (v: string) => void update({ gigot_base_url: v }),
});
const repoName = computed({
  get: () => config.value?.gigot_repo_name ?? "",
  set: (v: string) => void update({ gigot_repo_name: v }),
});

const token = ref("");
const saveToken = ref(true);

const cloning = ref(false);
const recloning = ref(false);
const confirmReclone = ref(false);

// Pre-clone confirm state — populated when LedgerSummary's pre-flight
// finds local managed changes that Clone would silently overwrite.
// Captured here so the dialog can name the count.
const confirmDirty = ref(false);
const dirtyChanged = ref<string[]>([]);

// Live bearer status — driven by the shared useCredentialStatus
// composable so the git side can adopt the same pattern for PAT
// status. The account key derives from the active profile + repo;
// when either changes the composable re-probes automatically.
const keychainAccount = computed(() => {
  const repo = repoName.value.trim();
  if (!repo || !profileFilename.value) return "";
  return accountFor("gigot", repo);
});

const { saved: bearerSaved, probing: bearerProbing, refresh: refreshBearerStatus }
  = useCredentialStatus(keychainAccount);

const bearerStatusKey = computed(() => {
  if (bearerProbing.value) return "";
  if (bearerSaved.value) return "workspace.collaboration.gigot.clone.bearer_saved";
  return "workspace.collaboration.gigot.clone.bearer_missing";
});

const bearerStatusVariant = computed<"ok" | undefined>(
  () => (bearerSaved.value ? "ok" : undefined),
);

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

// Refuse the unsafe rotation: typing a new token with Save=OFF while
// a bearer is already saved for this (profile, repo) would silently
// destroy the saved value (CredentialSvc.Get isn't exposed to the
// renderer, so we can't snapshot-and-restore). Force the user to
// either turn Save back on or clear the field.
const unsafeOverwrite = computed(
  () => token.value !== "" && !saveToken.value && bearerSaved.value,
);

const canAct = computed(
  () => !inFlight.value && configured.value && !unsafeOverwrite.value,
);

const actionDisabledHint = computed(() => {
  if (!configured.value || inFlight.value) return "";
  if (unsafeOverwrite.value) {
    return t("workspace.collaboration.gigot.clone.refuse_overwrite");
  }
  return "";
});

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

// Token handling around a single backend op. Returns a cleanup fn
// the caller must invoke in its `finally`.
//
// Safety invariant: never silently wipe a saved bearer the user
// didn't explicitly ask to overwrite. CredentialSvc deliberately
// doesn't expose Get to the renderer (the bearer never round-trips
// out of the OS keychain into JS), so we can't snapshot-and-restore
// the previous value. The UI guard (canAct) blocks the dangerous
// case — user types a new token, Save=OFF, AND a bearer already
// exists — so withTransientToken can assume one of:
//
//   a) No new token typed     → no keychain mutation.
//   b) New token + Save=ON    → Set overwrites old, permanently.
//   c) New token + no prior   → Set + Delete on cleanup (clean swap).
//
// Cases (b) and (c) match the user's explicit intent. The forbidden
// fourth case (new + Save=OFF + prior exists) is gated upstream.
async function withTransientToken(): Promise<() => Promise<void>> {
  const provided = token.value;
  if (provided === "") {
    return async () => {};
  }
  const account = keychainAccount.value;
  if (account === "") {
    return async () => {};
  }
  const persist = saveToken.value;
  await CredentialSvc.Set(account, provided);
  return async () => {
    if (persist) return;
    try {
      await CredentialSvc.Delete(account);
    } catch {
      /* delete failure is non-fatal — the op already succeeded */
    }
    void refreshBearerStatus();
  };
}

// Pre-flight: if the local ledger sees managed changes that haven't
// been pushed, Clone (a merge-aware pull) would silently overwrite
// them with server content. Surface a confirm dialog naming the
// count so the user can bail or choose to proceed with eyes open.
async function doClone() {
  if (!canAct.value) return;
  let pendingChanged: string[] = [];
  try {
    const summary = await GigotSvc.LedgerSummary();
    pendingChanged = (summary?.changed ?? []) as string[];
  } catch {
    /* If LedgerSummary fails (e.g. missing context), let PullLocal
       surface the same error — don't gate Clone on a probe. */
  }
  if (pendingChanged.length > 0) {
    dirtyChanged.value = pendingChanged;
    confirmDirty.value = true;
    return;
  }
  await runClone();
}

async function runClone() {
  if (!canAct.value) return;
  confirmDirty.value = false;
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
    window.dispatchEvent(new CustomEvent("formidable:context-reloaded"));
  } catch (err) {
    toast.error("workspace.collaboration.gigot.clone.error", [backendErrMessage(err)]);
  } finally {
    await cleanup();
    token.value = "";
    cloning.value = false;
    resetProgress();
    void refreshBearerStatus();
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
    window.dispatchEvent(new CustomEvent("formidable:context-reloaded"));
  } catch (err) {
    toast.error("workspace.collaboration.gigot.clone.reclone.error", [backendErrMessage(err)]);
  } finally {
    await cleanup();
    token.value = "";
    recloning.value = false;
    resetProgress();
    void refreshBearerStatus();
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.gigot.clone.info') }}</p>

  <div v-if="!configured && !inFlight" class="gigot-clone-note">
    {{ t('workspace.collaboration.gigot.clone.not_configured') }}
  </div>

  <FormSection>
    <FormRow :label="t('workspace.collaboration.gigot.clone.context_folder')">
      <FolderPathField
        v-model="contextFolder"
        placeholder="./Examples"
      />
    </FormRow>
    <FormRow :label="t('workspace.collaboration.gigot.clone.base_url')">
      <TextField
        v-model="baseURL"
        placeholder="https://gigot.example.com"
      />
    </FormRow>
    <FormRow :label="t('workspace.collaboration.gigot.clone.repo')">
      <TextField
        v-model="repoName"
        placeholder="addresses"
      />
    </FormRow>
    <FormRow
      :label="t('workspace.collaboration.gigot.clone.token')"
      :description="t('workspace.collaboration.gigot.clone.token_help')"
    >
      <TextField v-model="token" type="password" autocomplete="off" />
      <Badge v-if="bearerStatusKey" :variant="bearerStatusVariant">
        {{ t(bearerStatusKey, [profileFilename, repoName.trim() || '—']) }}
      </Badge>
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
      :title="actionDisabledHint"
      @click="doClone"
    >
      {{ cloning ? t('workspace.collaboration.gigot.clone.running') : t('workspace.collaboration.gigot.clone.button') }}
    </button>
    <button
      type="button"
      class="tool-btn danger"
      :disabled="!canAct"
      :title="actionDisabledHint"
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

  <ConfirmDialog
    :open="confirmDirty"
    :title="t('workspace.collaboration.gigot.clone.dirty_confirm_title')"
    :message="t('workspace.collaboration.gigot.clone.dirty_confirm_message', [String(dirtyChanged.length)])"
    :confirm-label="t('workspace.collaboration.gigot.clone.dirty_confirm_continue')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="confirmDirty = false"
    @confirm="runClone"
  />
</template>
