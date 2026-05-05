import { computed, ref } from "vue";
import { Service as ConfigSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";
import type { ProfileEntry, ProfileResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";
import { useConfig } from "./useConfig";

const profiles = ref<ProfileEntry[]>([]);
const lastError = ref<string>("");
let loaded = false;

const { profileFilename, switchProfile, reload: reloadConfig } = useConfig();

async function refresh(): Promise<void> {
  profiles.value = await ConfigSvc.ListUserProfiles();
  loaded = true;
}

async function ensureLoaded(): Promise<void> {
  if (!loaded) await refresh();
}

/** Switch to an existing profile filename (no creation). */
async function activate(filename: string): Promise<void> {
  await switchProfile(filename);
}

/**
 * Create a new profile by switching to a filename that doesn't exist
 * yet — the backend seeds defaults into it during LoadUserConfig.
 * Caller is responsible for validating the filename via isValidProfileFilename.
 */
async function create(filename: string): Promise<{ ok: boolean; code?: string; message?: string }> {
  // Refuse silently to overwrite an existing one. The user has the
  // list — switching is the explicit way to activate an existing
  // profile.
  await ensureLoaded();
  if (profiles.value.some((p) => p.value === filename)) {
    return { ok: false, code: "exists", message: "profile already exists" };
  }
  try {
    await switchProfile(filename);
    await refresh();
    return { ok: true };
  } catch (err) {
    return { ok: false, message: String(err) };
  }
}

async function remove(filename: string): Promise<ProfileResult> {
  const result = await ConfigSvc.DeleteUserProfile(filename);
  if (result?.success) {
    await refresh();
  }
  return result;
}

async function exportTo(filename: string, targetPath: string, overwrite: boolean): Promise<ProfileResult> {
  return ConfigSvc.ExportUserProfile(filename, targetPath, overwrite);
}

const FILENAME_RE = /^[a-z0-9-]+\.json$/;

/** Strict validation matching the original Formidable UI. */
export function isValidProfileFilename(name: string): boolean {
  return FILENAME_RE.test(name);
}

export function useProfiles() {
  if (!loaded) refresh().catch((err) => { lastError.value = String(err); });
  return {
    profiles,
    activeFilename: profileFilename,
    isActive: (name: string) => computed(() => name === profileFilename.value),
    refresh,
    activate,
    create,
    remove,
    exportTo,
    reloadConfig,
    lastError,
  };
}
