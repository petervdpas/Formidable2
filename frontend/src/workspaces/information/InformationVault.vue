<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useVault } from "../../composables/useVault";
import { useToast } from "../../composables/useToast";
import ConfirmDialog from "../../components/ConfirmDialog.vue";
import Modal from "../../components/Modal.vue";
import { FormRow, TextField, TextareaField } from "../../components/fields";

const { t } = useI18n();
const toast = useToast();

const {
  entries,
  loading,
  lastError,
  exists,
  unlocked,
  path,
  secretCount,
  hasEntries,
  foreignCount,
  minPasswordLength,
  autoLockMinutes,
  refresh,
  create,
  unlock,
  lock,
  setSecret,
  deleteSecret,
  changePassword,
  revealSecret,
  isWrongPassword,
} = useVault();

const newPassword = ref("");
const confirmPassword = ref("");
const unlockPassword = ref("");
const busy = ref(false);

const entryOpen = ref(false);
const entryCategory = ref("");
const entryKey = ref("");
const entryDescription = ref("");
const entryValue = ref("");
const entryIsNew = ref(true);
// Reveal applies to what is being typed here, not to anything read back from
// the vault. Stored values still never cross the boundary.
const showValue = ref(false);
const loadingValue = ref(false);

const pwOpen = ref(false);
const pwCurrent = ref("");
const pwNew = ref("");
const pwConfirm = ref("");

const deleteTarget = ref<{ category: string; key: string } | null>(null);
const deleteOpen = computed(() => deleteTarget.value !== null);
const deleteLabel = computed(() =>
  deleteTarget.value ? `${deleteTarget.value.category} / ${deleteTarget.value.key}` : "",
);

const statusKey = computed(() => {
  if (!exists.value) return "vault.status.absent";
  return unlocked.value ? "vault.status.unlocked" : "vault.status.locked";
});

const statusClass = computed(() => {
  if (!exists.value) return "absent";
  return unlocked.value ? "unlocked" : "locked";
});

const countText = computed(() =>
  secretCount.value === 1
    ? t("vault.secret_count_one")
    : t("vault.secret_count", [String(secretCount.value)]),
);

/** One rule, applied to both the create form and the change-password form.
 *  The minimum itself comes from the backend. */
function passwordProblem(pw: string, confirm: string): string {
  if (!pw && !confirm) return "";
  if (pw.length > 0 && pw.length < minPasswordLength.value) {
    return t("vault.create.too_short", [String(minPasswordLength.value)]);
  }
  if (confirm && pw !== confirm) return t("vault.create.mismatch");
  return "";
}

function passwordOK(pw: string, confirm: string): boolean {
  return minPasswordLength.value > 0 && pw.length >= minPasswordLength.value && pw === confirm;
}

const passwordError = computed(() => passwordProblem(newPassword.value, confirmPassword.value));
const canCreate = computed(() => !busy.value && passwordOK(newPassword.value, confirmPassword.value));

const pwError = computed(() => passwordProblem(pwNew.value, pwConfirm.value));
const canChangePassword = computed(
  () => !busy.value && !!pwCurrent.value && passwordOK(pwNew.value, pwConfirm.value),
);

const canSaveEntry = computed(
  () => !busy.value && entryCategory.value.trim().length > 0 && entryKey.value.trim().length > 0,
);

onMounted(() => {
  void refresh();
});

async function onCreate() {
  if (!canCreate.value) return;
  busy.value = true;
  const r = await create(newPassword.value);
  busy.value = false;
  newPassword.value = "";
  confirmPassword.value = "";
  if (r.ok) toast.success("vault.toast.created");
  else toast.error("vault.toast.create_failed", [r.message]);
}

async function onUnlock() {
  if (busy.value || !unlockPassword.value) return;
  busy.value = true;
  const r = await unlock(unlockPassword.value);
  busy.value = false;
  unlockPassword.value = "";
  if (r.ok) {
    toast.success("vault.toast.unlocked");
  } else if (isWrongPassword(r.message)) {
    toast.error("vault.toast.wrong_password");
  } else {
    toast.error("vault.toast.unlock_failed", [r.message]);
  }
}

async function onLock() {
  const r = await lock();
  if (r.ok) toast.success("vault.toast.locked");
  else toast.error("vault.toast.unlock_failed", [r.message]);
}

function openAdd() {
  entryIsNew.value = true;
  entryCategory.value = "";
  entryKey.value = "";
  entryDescription.value = "";
  entryValue.value = "";
  showValue.value = false;
  entryOpen.value = true;
}

function openChangePassword() {
  pwCurrent.value = "";
  pwNew.value = "";
  pwConfirm.value = "";
  pwOpen.value = true;
}

function closeChangePassword() {
  pwOpen.value = false;
  pwCurrent.value = "";
  pwNew.value = "";
  pwConfirm.value = "";
}

async function submitChangePassword() {
  if (!canChangePassword.value) return;
  busy.value = true;
  const r = await changePassword(pwCurrent.value, pwNew.value);
  busy.value = false;
  if (r.ok) {
    closeChangePassword();
    toast.success("vault.toast.password_changed");
  } else if (isWrongPassword(r.message)) {
    toast.error("vault.toast.wrong_password");
  } else {
    toast.error("vault.toast.password_failed", [r.message]);
  }
}

// Load the stored value so the dialog shows what is actually in the vault.
// An empty box on "replace" is indistinguishable from a vault that lost the
// secret, which is exactly how this looked before.
async function openReplace(e: { category: string; key: string; description?: string }) {
  entryIsNew.value = false;
  entryCategory.value = e.category;
  entryKey.value = e.key;
  entryDescription.value = e.description ?? "";
  entryValue.value = "";
  showValue.value = false;
  entryOpen.value = true;

  loadingValue.value = true;
  const r = await revealSecret(e.category, e.key);
  loadingValue.value = false;
  if (r.ok) entryValue.value = r.value;
  else toast.error("vault.toast.load_failed", [r.message]);
}

function closeEntry() {
  entryOpen.value = false;
  entryValue.value = "";
  showValue.value = false;
}

async function saveEntry() {
  if (!canSaveEntry.value) return;
  const category = entryCategory.value.trim();
  const key = entryKey.value.trim();
  busy.value = true;
  const r = await setSecret(category, key, entryValue.value, entryDescription.value.trim());
  busy.value = false;
  closeEntry();
  if (r.ok) toast.success("vault.toast.saved", [key]);
  else toast.error("vault.toast.save_failed", [r.message]);
}

async function confirmDelete() {
  const target = deleteTarget.value;
  deleteTarget.value = null;
  if (!target) return;
  const r = await deleteSecret(target.category, target.key);
  if (r.ok) toast.success("vault.toast.deleted", [target.key]);
  else toast.error("vault.toast.delete_failed", [r.message]);
}

/** The backend hands back an RFC 3339 timestamp; render it in the user's
 *  locale rather than shipping a date library for one column. */
function formatUpdated(value: unknown): string {
  if (!value) return t("vault.list.never");
  const d = new Date(String(value));
  if (Number.isNaN(d.getTime())) return t("vault.list.never");
  // Go marshals a zero time.Time as year 1 rather than null, so a record
  // written without a timestamp would otherwise render as "1/1/1".
  if (d.getUTCFullYear() <= 1) return t("vault.list.never");
  return d.toLocaleString();
}
</script>

<template>
  <p class="section-info">{{ t('vault.info') }}</p>

  <div class="vault-status">
    <div class="vault-status-line">
      <span class="vault-status-label">{{ t('vault.status_label') }}</span>
      <span :class="['vault-pill', statusClass]">{{ t(statusKey) }}</span>
      <span v-if="exists" class="muted small">{{ countText }}</span>
      <span class="vault-spacer"></span>
      <button class="tool-btn" type="button" :disabled="loading" @click="refresh">
        <i class="fa-solid fa-rotate" aria-hidden="true"></i>
        {{ t('vault.action.refresh') }}
      </button>
      <button v-if="unlocked" class="tool-btn" type="button" @click="openChangePassword">
        <i class="fa-solid fa-key" aria-hidden="true"></i>
        {{ t('vault.password.action') }}
      </button>
      <button v-if="unlocked" class="tool-btn" type="button" @click="onLock">
        <i class="fa-solid fa-lock" aria-hidden="true"></i>
        {{ t('vault.lock.submit') }}
      </button>
    </div>
    <div class="vault-status-line">
      <span class="vault-status-label">{{ t('vault.path_label') }}</span>
      <code class="vault-path">{{ path }}</code>
    </div>
    <p v-if="lastError" class="vault-error">
      <i class="fa-solid fa-circle-exclamation" aria-hidden="true"></i>
      {{ t('vault.toast.load_failed', [lastError]) }}
    </p>
  </div>

  <!-- No vault yet: create one. -->
  <section v-if="!exists" class="vault-gate">
    <h4>{{ t('vault.create.title') }}</h4>
    <p class="muted small">{{ t('vault.create.info') }}</p>

    <FormRow :label="t('vault.create.password')" label-for="vault-new-password">
      <TextField
        id="vault-new-password"
        v-model="newPassword"
        type="password"
        autocomplete="new-password"
        :placeholder="t('vault.create.placeholder')"
        :disabled="busy"
      />
    </FormRow>
    <FormRow
      :label="t('vault.create.confirm')"
      label-for="vault-confirm-password"
      :error="passwordError"
    >
      <TextField
        id="vault-confirm-password"
        v-model="confirmPassword"
        type="password"
        autocomplete="new-password"
        :disabled="busy"
        @keydown.enter="onCreate"
      />
    </FormRow>

    <p class="vault-warning">
      <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
      {{ t('vault.create.warning') }}
    </p>

    <div class="vault-actions">
      <button class="tool-btn primary" type="button" :disabled="!canCreate" @click="onCreate">
        {{ t('vault.create.submit') }}
      </button>
    </div>
  </section>

  <!-- Vault exists but is locked. -->
  <section v-else-if="!unlocked" class="vault-gate">
    <h4>{{ t('vault.unlock.title') }}</h4>
    <p class="muted small">{{ t('vault.unlock.info') }}</p>

    <FormRow :label="t('vault.unlock.password')" label-for="vault-unlock-password">
      <TextField
        id="vault-unlock-password"
        v-model="unlockPassword"
        type="password"
        autocomplete="current-password"
        :disabled="busy"
        @keydown.enter="onUnlock"
      />
    </FormRow>

    <div class="vault-actions">
      <button
        class="tool-btn primary"
        type="button"
        :disabled="busy || !unlockPassword"
        @click="onUnlock"
      >
        <i class="fa-solid fa-lock-open" aria-hidden="true"></i>
        {{ t('vault.unlock.submit') }}
      </button>
    </div>
    <p class="muted small">{{ t('vault.locked_notice') }}</p>
    <p class="muted small">{{ t('vault.auto_lock_hint', [String(autoLockMinutes)]) }}</p>
  </section>

  <!-- Unlocked: manage entries. Values are write-only by design. -->
  <section v-else class="vault-list">
    <div class="vault-list-header">
      <h4>{{ t('vault.list.title') }}</h4>
      <button class="tool-btn primary" type="button" @click="openAdd">
        <i class="fa-solid fa-plus" aria-hidden="true"></i>
        {{ t('vault.action.add') }}
      </button>
    </div>

    <p v-if="loading" class="muted small">{{ t('vault.list.loading') }}</p>
    <p v-else-if="!hasEntries" class="muted small">{{ t('vault.list.empty') }}</p>

    <table v-else class="vault-table">
      <thead>
        <tr>
          <th>{{ t('vault.list.category') }}</th>
          <th>{{ t('vault.list.key') }}</th>
          <th>{{ t('vault.list.updated') }}</th>
          <th class="vault-col-actions"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="e in entries" :key="e.slot">
          <td><code class="vault-entry-name">{{ e.category }}</code></td>
          <td>
            <code class="vault-entry-name">{{ e.key }}</code>
            <span v-if="e.description" class="muted small vault-entry-note">{{ e.description }}</span>
          </td>
          <td class="muted small">{{ formatUpdated(e.updated_utc) }}</td>
          <td class="vault-col-actions">
            <button
              class="tool-btn"
              type="button"
              :title="t('vault.action.replace')"
              @click="openReplace(e)"
            >
              <i class="fa-solid fa-pen" aria-hidden="true"></i>
            </button>
            <button
              class="tool-btn"
              type="button"
              :title="t('vault.action.delete')"
              @click="deleteTarget = { category: e.category, key: e.key }"
            >
              <i class="fa-solid fa-trash" aria-hidden="true"></i>
            </button>
          </td>
        </tr>
      </tbody>
    </table>

    <p v-if="foreignCount > 0" class="muted small">
      {{ t('vault.list.foreign', [String(foreignCount)]) }}
    </p>
    <p class="muted small">{{ t('vault.list.hint') }}</p>
  </section>

  <Modal
    :open="entryOpen"
    :title="t(entryIsNew ? 'vault.entry.title_add' : 'vault.entry.title_edit')"
    width="620px"
    @close="closeEntry"
  >
    <FormRow
      :label="t('vault.entry.category')"
      label-for="vault-entry-category"
      :description="entryIsNew ? t('vault.entry.category_hint') : ''"
    >
      <TextField
        id="vault-entry-category"
        v-model="entryCategory"
        :readonly="!entryIsNew"
        :disabled="busy"
      />
    </FormRow>
    <FormRow
      :label="t('vault.entry.key')"
      label-for="vault-entry-key"
      :description="entryIsNew ? t('vault.entry.key_hint') : ''"
    >
      <TextField
        id="vault-entry-key"
        v-model="entryKey"
        :readonly="!entryIsNew"
        :disabled="busy"
      />
    </FormRow>
    <FormRow
      :label="t('vault.entry.description')"
      label-for="vault-entry-description"
      :description="t('vault.entry.description_hint')"
    >
      <TextField id="vault-entry-description" v-model="entryDescription" :disabled="busy" />
    </FormRow>
    <FormRow
      :label="t('vault.entry.value')"
      label-for="vault-entry-value"
      :description="t('vault.entry.value_hint_multiline')"
    >
      <div class="vault-secret-input">
        <TextareaField
          id="vault-entry-value"
          v-model="entryValue"
          :rows="7"
          :disabled="busy || loadingValue"
          :class="{ 'vault-value-masked': !showValue }"
        />
        <button
          class="vault-reveal"
          type="button"
          :title="t(showValue ? 'vault.entry.hide' : 'vault.entry.show')"
          :aria-label="t(showValue ? 'vault.entry.hide' : 'vault.entry.show')"
          :aria-pressed="showValue"
          @click="showValue = !showValue"
        >
          <i :class="showValue ? 'fa-solid fa-eye-slash' : 'fa-solid fa-eye'" aria-hidden="true"></i>
        </button>
      </div>
      <p class="muted small vault-value-count">
        {{ t('vault.entry.length', [String(entryValue.length)]) }}
      </p>
    </FormRow>

    <template #footer>
      <button class="tool-btn" type="button" @click="closeEntry">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" :disabled="!canSaveEntry" @click="saveEntry">
        {{ t('vault.entry.save') }}
      </button>
    </template>
  </Modal>

  <Modal
    :open="pwOpen"
    :title="t('vault.password.title')"
    width="520px"
    @close="closeChangePassword"
  >
    <p class="muted small">{{ t('vault.password.info') }}</p>

    <FormRow :label="t('vault.password.current')" label-for="vault-pw-current">
      <TextField
        id="vault-pw-current"
        v-model="pwCurrent"
        type="password"
        autocomplete="current-password"
        :disabled="busy"
      />
    </FormRow>
    <FormRow :label="t('vault.password.new')" label-for="vault-pw-new">
      <TextField
        id="vault-pw-new"
        v-model="pwNew"
        type="password"
        autocomplete="new-password"
        :disabled="busy"
      />
    </FormRow>
    <FormRow
      :label="t('vault.password.confirm')"
      label-for="vault-pw-confirm"
      :error="pwError"
    >
      <TextField
        id="vault-pw-confirm"
        v-model="pwConfirm"
        type="password"
        autocomplete="new-password"
        :disabled="busy"
      />
    </FormRow>

    <p class="vault-warning">
      <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
      {{ t('vault.password.warning') }}
    </p>

    <template #footer>
      <button class="tool-btn" type="button" @click="closeChangePassword">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canChangePassword"
        @click="submitChangePassword"
      >
        {{ t('vault.password.submit') }}
      </button>
    </template>
  </Modal>

  <ConfirmDialog
    :open="deleteOpen"
    :title="t('vault.confirm.delete_title')"
    :message="t('vault.confirm.delete_message', [deleteLabel])"
    :confirm-label="t('vault.action.delete')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="deleteTarget = null"
    @confirm="confirmDelete"
  />
</template>
