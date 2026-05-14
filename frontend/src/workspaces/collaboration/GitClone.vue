<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import Badge from "../../components/Badge.vue";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
  FolderPathField,
} from "../../components/fields";
import { Service as GitSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as CredentialSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/credential";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { useConfig } from "../../composables/useConfig";
import { useCredentialAccount } from "../../composables/useCredentialAccount";
import { useCredentialStatus } from "../../composables/useCredentialStatus";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// One-shot Git clone form. PAT is held in a local ref only — never
// persisted, never written to config. The Wails service receives it
// in a single Clone() call and discards it after the response.
//
// On success we set git_root to the clone destination so the user
// lands back on Current Service with their new repo configured. The
// PAT field is cleared regardless of outcome (transient by design).
//
// PAT-saved status: surfaced live next to the PAT field via the
// shared useCredentialStatus composable — same pattern gigot's Clone
// uses. Account key is "<profile>:git:<remote_url>" so the badge
// re-probes as the user types the URL. Useful signal because git's
// post-clone Push/Pull/Fetch reuse the saved PAT via the keychain
// resolver — even when this form doesn't itself reuse it for the
// clone call (clone always uses what was typed).
const { t } = useI18n();
const { profileFilename, update } = useConfig();
const { accountFor } = useCredentialAccount();
const toast = useToast();

const url = ref("");
const dest = ref("");
const branch = ref("");
const pat = ref("");
const saveToken = ref(false);
const inFlight = ref(false);

const keychainAccount = computed(() => {
  const u = url.value.trim();
  if (!u || !profileFilename.value) return "";
  return accountFor("git", u);
});

const { saved: patSaved, probing: patProbing, refresh: refreshPatStatus }
  = useCredentialStatus(keychainAccount);

const patStatusKey = computed(() => {
  if (patProbing.value || keychainAccount.value === "") return "";
  return patSaved.value
    ? "workspace.collaboration.clone.pat_saved"
    : "workspace.collaboration.clone.pat_missing";
});

const patStatusVariant = computed<"ok" | undefined>(
  () => (patSaved.value ? "ok" : undefined),
);

const canClone = () =>
  !inFlight.value && url.value.trim() !== "" && dest.value.trim() !== "";

async function clone() {
  if (!canClone()) return;
  inFlight.value = true;
  try {
    // Clone with the user-picked dest (already passed through
    // MakeAppRootRelative by FolderPathField). Resolve back to an
    // absolute path before handing it to go-git, since the backend
    // op runs from the process cwd not AppRoot.
    const absDest = await SystemSvc.ResolveAbsolutePath(dest.value);
    const result = await GitSvc.Clone({
      url: url.value.trim(),
      dest: absDest || dest.value,
      branch: branch.value.trim(),
      pat: pat.value,
    });
    if (result?.dest) {
      // Store the human-friendly form in git_root, and the actual
      // checked-out branch in git_branch — backend reports it from
      // repo.Head() so this also covers the "no Branch input → got
      // remote default" case. Empty branch means detached HEAD;
      // skip the write so we don't blank the user's setting.
      const display = await SystemSvc.MakeAppRootRelative(result.dest);
      const patch: Record<string, string> = { git_root: display || result.dest };
      if (result.branch) patch.git_branch = result.branch;
      await update(patch);

      // If the user opted in, persist the PAT to the OS keychain.
      // Account name is namespaced "<profile>:git:<remote_url>" so
      // multiple profiles cloning the same repo each get their own
      // entry. Errors here don't undo the clone — we surface them
      // as a separate toast and let the user retry the save.
      if (saveToken.value && pat.value !== "") {
        try {
          const account = accountFor("git", url.value.trim());
          await CredentialSvc.Set(account, pat.value);
          void refreshPatStatus();
        } catch (err) {
          toast.error("workspace.collaboration.clone.save_token_error", [backendErrMessage(err)]);
        }
      }

      toast.success("workspace.collaboration.clone.success");
      // Reset the form.
      url.value = "";
      dest.value = "";
      branch.value = "";
      saveToken.value = false;
    }
  } catch (err) {
    toast.error("workspace.collaboration.clone.error", [backendErrMessage(err)]);
  } finally {
    // PAT is always wiped — even on error, we don't want it to
    // linger in component state across navigation.
    pat.value = "";
    inFlight.value = false;
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.clone.info') }}</p>

  <FormSection>
    <FormRow :label="t('workspace.collaboration.clone.url')">
      <TextField v-model="url" placeholder="https://github.com/owner/repo.git" />
    </FormRow>
    <FormRow
      :label="t('workspace.collaboration.clone.pat')"
      :description="t('workspace.collaboration.clone.pat_help')"
    >
      <TextField v-model="pat" type="password" autocomplete="off" />
      <Badge v-if="patStatusKey" :variant="patStatusVariant">
        {{ t(patStatusKey, [profileFilename, url.trim()]) }}
      </Badge>
    </FormRow>
    <FormSwitchRow
      :label="t('workspace.collaboration.clone.save_token')"
      :description="t('workspace.collaboration.clone.save_token_help')"
      v-model="saveToken"
    />
    <FormRow :label="t('workspace.collaboration.clone.dest')">
      <FolderPathField v-model="dest" placeholder="/path/to/empty/folder" />
    </FormRow>
    <FormRow :label="t('workspace.collaboration.clone.branch')">
      <TextField v-model="branch" placeholder="main" />
    </FormRow>
  </FormSection>

  <div class="git-clone-actions">
    <button
      type="button"
      class="tool-btn primary"
      :disabled="!canClone()"
      @click="clone"
    >
      {{ inFlight ? t('workspace.collaboration.clone.running') : t('workspace.collaboration.clone.button') }}
    </button>
  </div>
</template>
