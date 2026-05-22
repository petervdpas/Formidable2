import { onMounted, ref, watch, type ComputedRef, type Ref } from "vue";
import { Service as CredentialSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/credential";

// useCredentialStatus probes the OS keychain reactively for a given
// account key and exposes a simple "saved? / still probing?" pair.
// Re-probes on mount and whenever the account key changes - useful
// when the key derives from reactive config (e.g. the active
// profile filename and the current repo name).
//
// CredentialSvc.Get is deliberately not exposed to the renderer (the
// stored secret must never round-trip out of the keychain into JS),
// so consumers can only learn *whether* a secret is saved, not what
// it is. This composable wraps the .Has() lookup.
//
// Designed for both the gigot Clone surface (Subscription Token
// status) and the git Clone surface (PAT-for-this-remote status).
// Lives in composables/ so neither backend takes a direct dependency
// on the other's per-backend Vue files.

export interface CredentialStatus {
  /** True when CredentialSvc.Has reports an entry exists for the
   *  current accountKey. False when the entry is absent, the key is
   *  empty, or a recent probe threw. */
  saved: Ref<boolean>;
  /** True while a probe is in flight. Lets callers gate "Save now"
   *  UI without racing the initial probe. */
  probing: Ref<boolean>;
  /** Force an immediate re-probe - call after CredentialSvc.Set /
   *  Delete to keep the badge in sync. */
  refresh: () => Promise<void>;
}

export function useCredentialStatus(accountKey: ComputedRef<string>): CredentialStatus {
  const saved = ref(false);
  const probing = ref(false);

  async function refresh() {
    const account = accountKey.value;
    if (account === "") {
      saved.value = false;
      return;
    }
    probing.value = true;
    try {
      const res = await CredentialSvc.Has(account);
      saved.value = !!res?.found;
    } catch {
      saved.value = false;
    } finally {
      probing.value = false;
    }
  }

  onMounted(() => void refresh());
  watch(accountKey, () => void refresh());

  return { saved, probing, refresh };
}
