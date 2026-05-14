<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
} from "../../components/fields";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import { Service as CredentialSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/credential";
import { useConfig } from "../../composables/useConfig";
import { useCredentialAccount } from "../../composables/useCredentialAccount";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// One-shot GiGot connect form. The subscription bearer is held in a
// local ref only — never persisted to config, never round-trips
// through the JSON profile. On Connect the addressing fields land in
// config (gigot_base_url / gigot_repo_name) and the token lands in
// the OS keychain under "<profile>:gigot:<repoName>". A Ping verifies
// the credentials reach the server before the user walks away.
//
// "Save bearer" defaults ON: unlike a one-off git Clone, gigot is
// sync-first and every subsequent op needs the token. The opt-out is
// for users who want to verify a connection without persisting the
// secret across sessions.

const { t } = useI18n();
const { config, update } = useConfig();
const { accountFor } = useCredentialAccount();
const toast = useToast();

const baseURL = ref(config.value?.gigot_base_url ?? "");
const repoName = ref(config.value?.gigot_repo_name ?? "");
const token = ref("");
const saveToken = ref(true);
const inFlight = ref(false);

const canConnect = () =>
  !inFlight.value
  && baseURL.value.trim() !== ""
  && repoName.value.trim() !== ""
  && token.value !== "";

async function connect() {
  if (!canConnect()) return;
  inFlight.value = true;
  const url = baseURL.value.trim();
  const repo = repoName.value.trim();
  const account = accountFor("gigot", repo);
  let savedKeychain = false;
  try {
    await update({ gigot_base_url: url, gigot_repo_name: repo });
    await CredentialSvc.Set(account, token.value);
    savedKeychain = true;

    const health = await GigotSvc.Ping();
    if (!health) {
      throw new Error(t("workspace.collaboration.gigot.connect.unhealthy"));
    }

    if (!saveToken.value) {
      try {
        await CredentialSvc.Delete(account);
      } catch {
        /* keychain delete failure is non-fatal for the connection test */
      }
    }
    toast.success("workspace.collaboration.gigot.connect.success");
    saveToken.value = true;
  } catch (err) {
    if (savedKeychain && !saveToken.value) {
      try {
        await CredentialSvc.Delete(account);
      } catch {
        /* ignored — same rationale as the happy-path branch */
      }
    }
    toast.error("workspace.collaboration.gigot.connect.error", [backendErrMessage(err)]);
  } finally {
    token.value = "";
    inFlight.value = false;
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.gigot.connect.info') }}</p>

  <FormSection>
    <FormRow :label="t('workspace.collaboration.gigot.connect.base_url')">
      <TextField v-model="baseURL" placeholder="https://gigot.example" />
    </FormRow>
    <FormRow :label="t('workspace.collaboration.gigot.connect.repo')">
      <TextField v-model="repoName" placeholder="addresses" />
    </FormRow>
    <FormRow
      :label="t('workspace.collaboration.gigot.connect.token')"
      :description="t('workspace.collaboration.gigot.connect.token_help')"
    >
      <TextField v-model="token" type="password" autocomplete="off" />
    </FormRow>
    <FormSwitchRow
      :label="t('workspace.collaboration.gigot.connect.save_token')"
      :description="t('workspace.collaboration.gigot.connect.save_token_help')"
      v-model="saveToken"
    />
  </FormSection>

  <div class="gigot-connect-actions">
    <button
      type="button"
      class="tool-btn primary"
      :disabled="!canConnect()"
      @click="connect"
    >
      {{ inFlight ? t('workspace.collaboration.gigot.connect.running') : t('workspace.collaboration.gigot.connect.button') }}
    </button>
  </div>
</template>
