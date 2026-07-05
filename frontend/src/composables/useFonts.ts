import { ref, computed } from "vue";
import {
  Service as FontsSvc,
  type FontInfo,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/fonts";
import { backendErrMessage } from "../utils/backendError";

// Module-scope refs so the Fonts panel keeps its list when the user clicks to a
// sibling Information section and back (mirrors usePDFCoverImages).
const fonts = ref<FontInfo[]>([]);
const fontFaceCss = ref<string>("");
const loading = ref<boolean>(false);
const lastError = ref<string>("");

async function refresh(): Promise<void> {
  loading.value = true;
  lastError.value = "";
  try {
    fonts.value = (await FontsSvc.ListFonts()) ?? [];
    fontFaceCss.value = (await FontsSvc.FontFaceCSS()) ?? "";
  } catch (err) {
    lastError.value = backendErrMessage(err);
    fonts.value = [];
  } finally {
    loading.value = false;
  }
}

function isSeed(filename: string): boolean {
  return fonts.value.some((f) => f.filename === filename && f.isSeed);
}

type Result = { ok: boolean; message: string };

async function uploadFile(file: File): Promise<Result> {
  try {
    const base64 = await readFileAsBase64(file);
    await FontsSvc.SaveFont(file.name, base64);
    await refresh();
    return { ok: true, message: "" };
  } catch (err) {
    return { ok: false, message: backendErrMessage(err) };
  }
}

async function removeFont(name: string): Promise<Result> {
  try {
    await FontsSvc.DeleteFont(name);
    await refresh();
    return { ok: true, message: "" };
  } catch (err) {
    return { ok: false, message: backendErrMessage(err) };
  }
}

async function restoreDefaults(): Promise<Result> {
  try {
    await FontsSvc.RestoreDefaultFonts();
    await refresh();
    return { ok: true, message: "" };
  } catch (err) {
    return { ok: false, message: backendErrMessage(err) };
  }
}

// readFileAsBase64 returns the file body as a bare base64 string (no
// data:…;base64, prefix); the backend strips that prefix defensively anyway.
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

const hasFonts = computed(() => fonts.value.length > 0);
const hasSeeds = computed(() => fonts.value.some((f) => f.isSeed));

export function useFonts() {
  return {
    fonts,
    fontFaceCss,
    loading,
    lastError,
    hasFonts,
    hasSeeds,
    refresh,
    uploadFile,
    removeFont,
    restoreDefaults,
    isSeed,
  };
}
