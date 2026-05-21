import { ref, computed } from "vue";
import * as PdfSvc from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/service";
import type { CoverImageDescriptor } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/models";
import { backendErrMessage } from "../utils/backendError";

// Module-scope refs so the panel keeps its list when the user clicks
// back to a sibling Information section and returns — matching the
// pattern in usePDFCovers.ts.
const images = ref<CoverImageDescriptor[]>([]);
const loading = ref<boolean>(false);
const lastError = ref<string>("");

async function refresh(): Promise<void> {
  loading.value = true;
  lastError.value = "";
  try {
    images.value = (await PdfSvc.ListCoverImages()) ?? [];
  } catch (err) {
    lastError.value = backendErrMessage(err);
    images.value = [];
  } finally {
    loading.value = false;
  }
}

function isSeed(name: string): boolean {
  return images.value.some((img) => img.name === name && img.isSeed);
}

type Result = { ok: boolean; message: string };

async function uploadFile(file: File): Promise<Result> {
  try {
    const base64 = await readFileAsBase64(file);
    await PdfSvc.SaveCoverImage(file.name, base64);
    await refresh();
    return { ok: true, message: "" };
  } catch (err) {
    return { ok: false, message: backendErrMessage(err) };
  }
}

async function removeImage(name: string): Promise<Result> {
  try {
    await PdfSvc.DeleteCoverImage(name);
    await refresh();
    return { ok: true, message: "" };
  } catch (err) {
    return { ok: false, message: backendErrMessage(err) };
  }
}

async function loadDataURL(name: string): Promise<string> {
  const base64 = await PdfSvc.LoadCoverImage(name);
  const mime = mimeFromName(name);
  return `data:${mime};base64,${base64}`;
}

function mimeFromName(name: string): string {
  const lower = name.toLowerCase();
  if (lower.endsWith(".svg")) return "image/svg+xml";
  if (lower.endsWith(".png")) return "image/png";
  if (lower.endsWith(".jpg") || lower.endsWith(".jpeg")) return "image/jpeg";
  if (lower.endsWith(".gif")) return "image/gif";
  if (lower.endsWith(".webp")) return "image/webp";
  return "application/octet-stream";
}

// readFileAsBase64 returns the file body as a bare base64 string
// (no `data:…;base64,` prefix). The backend strips that prefix
// defensively, but keeping the wire form pure base64 makes traces
// easier to read.
function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("read failed"));
    reader.onload = () => {
      const result = reader.result;
      if (typeof result !== "string") {
        reject(new Error("FileReader returned non-string"));
        return;
      }
      const idx = result.indexOf(",");
      resolve(idx >= 0 ? result.slice(idx + 1) : result);
    };
    reader.readAsDataURL(file);
  });
}

const hasImages = computed(() => images.value.length > 0);

export function usePDFCoverImages() {
  return {
    images,
    loading,
    lastError,
    hasImages,
    refresh,
    uploadFile,
    removeImage,
    loadDataURL,
    isSeed,
  };
}
