import { computed, ref } from "vue";
import { Events } from "@wailsio/runtime";
import {
  Service as VaultSvc,
  type CatalogEntry,
  type Policy,
  type Status,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/vault";
import { backendErrMessage } from "../utils/backendError";

// Module-scope state so the panel keeps what it knows when the user clicks to a
// sibling Information section and back (mirrors useFonts / usePDFCoverImages).
const status = ref<Status | null>(null);
const entries = ref<CatalogEntry[]>([]);
const loading = ref<boolean>(false);
const lastError = ref<string>("");

// The password rules come from the backend, never from a constant in here.
// A minimum restated in Vue drifts the first time Go changes it.
const policy = ref<Policy | null>(null);

// The vault locks itself after an idle timeout, and the backend is the only
// thing that knows when. Without this subscription the panel keeps showing
// "Unlocked" until the user's next action fails, which is the worst moment to
// discover it. Subscribed once at module scope, matching the state above.
let subscribed = false;

function subscribeOnce(): void {
  if (subscribed) return;
  subscribed = true;
  Events.On("vault:locked", () => {
    void refresh();
  });
}

type Result = { ok: boolean; message: string };

function ok(): Result {
  return { ok: true, message: "" };
}

function failed(err: unknown): Result {
  return { ok: false, message: backendErrMessage(err) };
}

/** The Go sentinel for a wrong master password, so the panel can say the one
 *  thing the user actually needs to hear instead of echoing a Go error. */
const WRONG_PASSWORD = "vault: invalid master password";

function isWrongPassword(message: string): boolean {
  return message.includes(WRONG_PASSWORD);
}

/** Refresh status, and the entry list too when the vault is readable. The list
 *  needs no unlock: entry names are filenames, only values are sealed. */
async function refresh(): Promise<void> {
  loading.value = true;
  lastError.value = "";
  try {
    if (!policy.value) policy.value = await VaultSvc.VaultPolicy();
    status.value = await VaultSvc.VaultStatus();
    // Listing needs the vault open: identity lives inside the ciphertext, so a
    // locked vault genuinely cannot enumerate what it holds.
    entries.value = status.value?.unlocked ? ((await VaultSvc.ListSecrets()) ?? []) : [];
  } catch (err) {
    lastError.value = backendErrMessage(err);
    entries.value = [];
  } finally {
    loading.value = false;
  }
}

async function create(masterPassword: string): Promise<Result> {
  try {
    await VaultSvc.InitializeVault(masterPassword);
    await refresh();
    return ok();
  } catch (err) {
    return failed(err);
  }
}

async function unlock(masterPassword: string): Promise<Result> {
  try {
    await VaultSvc.UnlockVault(masterPassword);
    await refresh();
    return ok();
  } catch (err) {
    return failed(err);
  }
}

async function lock(): Promise<Result> {
  try {
    await VaultSvc.LockVault();
    await refresh();
    return ok();
  } catch (err) {
    return failed(err);
  }
}

async function setSecret(
  category: string,
  key: string,
  value: string,
  description: string,
): Promise<Result> {
  try {
    await VaultSvc.SetSecret(category, key, value, description);
    await refresh();
    return ok();
  } catch (err) {
    return failed(err);
  }
}

/** Read a stored value back. Separate from Result because "" is a legitimate
 *  secret, so the value has to travel beside the ok flag rather than in it. */
async function revealSecret(category: string, key: string): Promise<Result & { value: string }> {
  try {
    const value = await VaultSvc.RevealSecret(category, key);
    return { ...ok(), value };
  } catch (err) {
    return { ...failed(err), value: "" };
  }
}

async function changePassword(oldPassword: string, newPassword: string): Promise<Result> {
  try {
    await VaultSvc.ChangeMasterPassword(oldPassword, newPassword);
    await refresh();
    return ok();
  } catch (err) {
    return failed(err);
  }
}

async function deleteSecret(category: string, key: string): Promise<Result> {
  try {
    await VaultSvc.DeleteSecret(category, key);
    await refresh();
    return ok();
  } catch (err) {
    return failed(err);
  }
}

export function useVault() {
  subscribeOnce();

  return {
    status,
    entries,
    loading,
    lastError,

    policy,
    minPasswordLength: computed<number>(() => policy.value?.min_password_length ?? 0),
    autoLockMinutes: computed<number>(() => policy.value?.auto_lock_minutes ?? 0),

    exists: computed<boolean>(() => status.value?.exists === true),
    unlocked: computed<boolean>(() => status.value?.unlocked === true),
    path: computed<string>(() => status.value?.path ?? ""),
    secretCount: computed<number>(() => status.value?.secrets ?? 0),
    hasEntries: computed<boolean>(() => entries.value.length > 0),
    foreignCount: computed<number>(() => status.value?.foreign ?? 0),

    refresh,
    create,
    unlock,
    lock,
    setSecret,
    deleteSecret,
    changePassword,
    revealSecret,
    isWrongPassword,
  };
}
