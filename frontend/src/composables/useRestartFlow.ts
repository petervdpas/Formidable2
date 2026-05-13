import { ref } from "vue";
import { i18n } from "../i18n";
import { Service as System } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { backendErrMessage } from "../utils/backendError";

// useRestartFlow centralises the "confirm dialog → optional pre-step
// → System.Restart() → AlertDialog on failure" pattern. Settings uses
// it for plain restart-after-config-change; Profiles uses it with a
// `before` hook that flips .boot.json so the relaunched process boots
// the new profile. Per-call factory (not module-singleton) so two
// workspaces mounted concurrently don't share dialog state.

export interface RestartRequest {
  /** Async work to run between user confirmation and the actual
   *  System.Restart() call. Throw to short-circuit the restart and
   *  surface the error in the AlertDialog. */
  before?: () => void | Promise<void>;
  /** i18n key for the failure-AlertDialog message. The thrown error
   *  is passed as `{0}`, already run through backendErrMessage so
   *  Wails JSON envelopes don't leak. Defaults to settings.apply_error
   *  to keep the original behaviour for callers that don't override. */
  errorKey?: string;
}

export function useRestartFlow() {
  const confirmOpen = ref(false);
  const errorOpen = ref(false);
  const errorMessage = ref("");
  // Captured at request() time so a click-to-cancel + click-to-request
  // again can't accidentally reuse a stale before-hook.
  let pending: RestartRequest | null = null;

  function request(req: RestartRequest = {}) {
    pending = req;
    confirmOpen.value = true;
  }

  function cancel() {
    confirmOpen.value = false;
    pending = null;
  }

  async function confirm() {
    confirmOpen.value = false;
    const req = pending;
    pending = null;
    try {
      if (req?.before) await req.before();
      await System.Restart();
    } catch (err) {
      const key = req?.errorKey ?? "settings.apply_error";
      errorMessage.value = i18n.global.t(key, [backendErrMessage(err)]);
      errorOpen.value = true;
    }
  }

  function dismissError() {
    errorOpen.value = false;
  }

  return {
    confirmOpen,
    errorOpen,
    errorMessage,
    request,
    cancel,
    confirm,
    dismissError,
  };
}
