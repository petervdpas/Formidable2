import { useConfig } from "./useConfig";

// Backends that participate in the OS keychain. Add new entries
// here when GiGot (or any future backend) lands its sync flow so
// the account string stays type-safe.
export type CredentialBackend = "git" | "gigot";

/**
 * Canonical credential account name = `<profile>:<backend>:<identifier>`.
 *
 * - **Profile** namespacing means two profiles cloning the same repo
 *   each get their own keychain entry - the same user often has
 *   "personal" and "work" profiles with different PATs.
 * - **Backend** namespacing leaves room for GiGot's subscription
 *   token alongside Git PATs without collision.
 * - **Identifier** is per-backend: the remote URL for Git, the GiGot
 *   repo name for GiGot.
 *
 * The convention is duplicated in the Go `credential` package
 * comment; sync operations on the backend (which read the secret
 * directly from the keychain) must produce the same string. Keep
 * these two definitions in sync if either side changes.
 */
export function useCredentialAccount() {
  const { profileFilename } = useConfig();

  function accountFor(backend: CredentialBackend, identifier: string): string {
    if (!profileFilename.value) {
      // No active profile means useConfig hasn't loaded yet (boot
      // race) - saving under ":<backend>:<id>" would put a stranded
      // entry in the keychain that no profile can find later.
      throw new Error("credential account requires an active profile");
    }
    return `${profileFilename.value}:${backend}:${identifier}`;
  }

  return { accountFor };
}
