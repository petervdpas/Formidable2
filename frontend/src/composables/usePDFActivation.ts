import { ref, onMounted } from "vue";
import * as PdfSvc from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/service";
import type {
  Status,
  ActivateOpts,
  ProbeResult,
  ExportTelemetrySnapshot,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/models";
import { backendErrMessage } from "../utils/backendError";

// Module-scope singletons. Activation state only changes through this
// composable's actions (no external mutator), so a refresh-on-action
// strategy is enough — no polling timer.
const status = ref<Status | null>(null);
const lastError = ref<string>("");
const lastExport = ref<ExportTelemetrySnapshot | null>(null);
// Asset server's bound host:port (or "" when no server). Cached
// once at mount; the listener is process-lifetime so it doesn't
// change between renders.
const assetServerAddr = ref<string>("");
let assetServerLoaded = false;

async function refresh() {
  try {
    status.value = await PdfSvc.GetStatus();
  } catch (err) {
    lastError.value = backendErrMessage(err);
  }
}

async function refreshLastExport() {
  try {
    lastExport.value = await PdfSvc.LastExport();
  } catch (err) {
    lastError.value = backendErrMessage(err);
  }
}

async function refreshAssetServer() {
  try {
    assetServerAddr.value = await PdfSvc.AssetServerAddr();
    assetServerLoaded = true;
  } catch {
    assetServerAddr.value = "";
    assetServerLoaded = true;
  }
}

export function usePDFActivation() {
  onMounted(() => {
    if (status.value === null) {
      void refresh();
    }
    if (lastExport.value === null) {
      void refreshLastExport();
    }
    if (!assetServerLoaded) {
      void refreshAssetServer();
    }
  });

  async function probe(): Promise<
    { ok: true; result: ProbeResult } | { ok: false; message: string }
  > {
    lastError.value = "";
    try {
      const result = await PdfSvc.ProbeChrome();
      return { ok: true as const, result };
    } catch (err) {
      const message = backendErrMessage(err);
      lastError.value = message;
      return { ok: false as const, message };
    }
  }

  async function activate(opts: ActivateOpts = {} as ActivateOpts) {
    lastError.value = "";
    try {
      const s = await PdfSvc.Activate(opts);
      status.value = s;
      return { ok: true as const, status: s };
    } catch (err) {
      const message = backendErrMessage(err);
      lastError.value = message;
      return { ok: false as const, message };
    }
  }

  async function deactivate() {
    lastError.value = "";
    try {
      await PdfSvc.Deactivate();
      await refresh();
      return { ok: true as const };
    } catch (err) {
      const message = backendErrMessage(err);
      lastError.value = message;
      return { ok: false as const, message };
    }
  }

  async function setExportDir(path: string) {
    lastError.value = "";
    try {
      const s = await PdfSvc.SetExportDir(path);
      status.value = s;
      return { ok: true as const, status: s };
    } catch (err) {
      const message = backendErrMessage(err);
      lastError.value = message;
      return { ok: false as const, message };
    }
  }

  return {
    status,
    lastError,
    lastExport,
    assetServerAddr,
    refresh,
    refreshLastExport,
    refreshAssetServer,
    probe,
    activate,
    deactivate,
    setExportDir,
  };
}
