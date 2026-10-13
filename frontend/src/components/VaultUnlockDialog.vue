<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { FormRow, TextField } from "./fields";
import { useVault } from "../composables/useVault";
import { useVaultUnlockPrompt } from "../composables/useVaultUnlockPrompt";

// VaultUnlockDialog - the prompt behind askVaultUnlock(). It asks for the
// master password where the user is, instead of sending them to Information >
// Vault and back. A wrong password keeps the dialog open with the reason.

const { t } = useI18n();
const { unlock, isWrongPassword } = useVault();
const { open, settle } = useVaultUnlockPrompt();

const password = ref<string>("");
const busy = ref<boolean>(false);
const error = ref<string>("");

watch(open, (isOpen) => {
  if (isOpen) {
    password.value = "";
    error.value = "";
  }
});

async function submit(): Promise<void> {
  if (busy.value || !password.value) return;
  busy.value = true;
  error.value = "";
  const r = await unlock(password.value);
  busy.value = false;
  password.value = "";
  if (r.ok) {
    settle(true);
    return;
  }
  error.value = isWrongPassword(r.message)
    ? t("vault.toast.wrong_password")
    : t("vault.toast.unlock_failed", [r.message]);
}

function cancel(): void {
  if (busy.value) return;
  settle(false);
}
</script>

<template>
  <Modal :open="open" :title="t('vault.unlock.title')" width="460px" elevated @close="cancel">
    <p class="muted small">{{ t("vault.prompt.info") }}</p>
    <FormRow :label="t('vault.unlock.password')" label-for="vault-prompt-password">
      <TextField
        id="vault-prompt-password"
        v-model="password"
        type="password"
        autocomplete="current-password"
        :disabled="busy"
        @keydown.enter="submit"
      />
    </FormRow>
    <p v-if="error" class="vault-prompt-error small">{{ error }}</p>

    <template #footer>
      <button class="tool-btn" type="button" :disabled="busy" @click="cancel">
        {{ t("common.cancel") }}
      </button>
      <button class="tool-btn primary" type="button" :disabled="busy || !password" @click="submit">
        <i class="fa-solid fa-lock-open" aria-hidden="true"></i>
        {{ t("vault.unlock.submit") }}
      </button>
    </template>
  </Modal>
</template>
