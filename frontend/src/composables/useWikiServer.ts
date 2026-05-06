import { ref, onUnmounted } from "vue";
import { Service as WikiSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/wiki";
import type { ServerStatus } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/wiki";

// Module-scope singleton — at most one Information panel watches the
// server at a time, but if a future workspace polls too they share
// the same source. Polling is cheap (Status reads a memory snapshot)
// but the singleton avoids redundant timers.
const status = ref<ServerStatus | null>(null);
const lastError = ref<string>("");

let pollHandle: number | null = null;
let pollers = 0;

async function refresh() {
  try {
    status.value = await WikiSvc.GetServerStatus();
  } catch (err) {
    lastError.value = String(err);
  }
}

function startPolling(intervalMs = 1000) {
  if (pollHandle !== null) return;
  pollHandle = window.setInterval(refresh, intervalMs);
}

function stopPolling() {
  if (pollHandle === null) return;
  window.clearInterval(pollHandle);
  pollHandle = null;
}

export function useWikiServer() {
  pollers += 1;
  void refresh();
  startPolling();

  onUnmounted(() => {
    pollers -= 1;
    if (pollers <= 0) {
      pollers = 0;
      stopPolling();
    }
  });

  async function start() {
    lastError.value = "";
    try {
      await WikiSvc.StartServer();
      await refresh();
      return { ok: true as const };
    } catch (err) {
      lastError.value = String(err);
      return { ok: false as const, message: String(err) };
    }
  }

  async function stop() {
    lastError.value = "";
    try {
      await WikiSvc.StopServer();
      await refresh();
      return { ok: true as const };
    } catch (err) {
      lastError.value = String(err);
      return { ok: false as const, message: String(err) };
    }
  }

  async function openBrowser() {
    lastError.value = "";
    try {
      await WikiSvc.OpenInBrowser();
      return { ok: true as const };
    } catch (err) {
      lastError.value = String(err);
      return { ok: false as const, message: String(err) };
    }
  }

  async function openInternal() {
    lastError.value = "";
    try {
      await WikiSvc.OpenInternalWiki();
      return { ok: true as const };
    } catch (err) {
      lastError.value = String(err);
      return { ok: false as const, message: String(err) };
    }
  }

  async function openAPIDocsInBrowser() {
    lastError.value = "";
    try {
      await WikiSvc.OpenAPIDocsInBrowser();
      return { ok: true as const };
    } catch (err) {
      lastError.value = String(err);
      return { ok: false as const, message: String(err) };
    }
  }

  async function openAPIDocsInWindow() {
    lastError.value = "";
    try {
      await WikiSvc.OpenAPIDocsInWindow();
      return { ok: true as const };
    } catch (err) {
      lastError.value = String(err);
      return { ok: false as const, message: String(err) };
    }
  }

  return {
    status,
    lastError,
    refresh,
    start,
    stop,
    openBrowser,
    openInternal,
    openAPIDocsInBrowser,
    openAPIDocsInWindow,
  };
}
