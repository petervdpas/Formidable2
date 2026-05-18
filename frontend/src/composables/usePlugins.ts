import { computed, ref } from "vue";
import {
  Service as PluginSvc,
  type ListResult,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";

// Module-scope singleton — at most one plugin selected across the
// app, mirroring the useTemplates pattern. Sidebar list, currently
// selected id, and a "loaded once" flag.
const plugins = ref<ListResult[]>([]);
const selectedID = ref<string>("");
let loaded = false;

// Workspace IDs a plugin manifest may attach to. Sourced from the
// backend (PluginSvc.ListWorkspaces) so the dropdown in the manifest
// editor doesn't drift from the Go enum. Cached once per session —
// the enum is closed and doesn't change at runtime.
const workspaceIDs = ref<string[]>([]);
let workspacesLoaded = false;
async function refreshWorkspaces(): Promise<void> {
  workspaceIDs.value = await PluginSvc.ListWorkspaces();
  workspacesLoaded = true;
}

async function refresh(): Promise<void> {
  plugins.value = await PluginSvc.Refresh();
  loaded = true;
}

// See useTemplates: pull/clone/reclone fires this event after writing
// to the context folder. Re-read the plugin list from disk so the
// sidebar reflects upstream changes without an app restart.
if (typeof window !== "undefined") {
  window.addEventListener("formidable:context-reloaded", () => {
    if (loaded) void refresh();
  });
}

// Backend's validID rule: lowercase ASCII letters, digits, dash,
// underscore. Empty rejected. Mirrored client-side so the Create
// dialog can show a fast inline error before the round-trip.
const PLUGIN_ID_RE = /^[a-z0-9_-]+$/;
export function isValidPluginID(id: string): boolean {
  return PLUGIN_ID_RE.test(id);
}

// Surface the closed set of error reasons the create flow cares
// about (exists vs invalid vs other) so the caller can branch on a
// stable code rather than parsing error text. The backend wraps
// these as ErrPluginExists / ErrManifestInvalid; we sniff the
// message for a stable substring.
type CreateOutcome =
  | { ok: true }
  | { ok: false; code: "exists" | "invalid" | "exception"; message?: string };

async function create(id: string): Promise<CreateOutcome> {
  if (!isValidPluginID(id)) {
    return { ok: false, code: "invalid" };
  }
  try {
    plugins.value = await PluginSvc.Create(id);
    selectedID.value = id;
    return { ok: true };
  } catch (err) {
    const msg = String(err);
    if (msg.includes("already exists")) {
      return { ok: false, code: "exists", message: msg };
    }
    if (msg.includes("invalid manifest")) {
      return { ok: false, code: "invalid", message: msg };
    }
    return { ok: false, code: "exception", message: msg };
  }
}

async function remove(id: string): Promise<{ ok: boolean; message?: string }> {
  try {
    plugins.value = await PluginSvc.Delete(id);
    if (selectedID.value === id) selectedID.value = "";
    return { ok: true };
  } catch (err) {
    return { ok: false, message: String(err) };
  }
}

// Export/import wraps the Wails archive surface. The backend bundles
// <PluginsDir>/<id>/* into a zip on export, and unpacks (refusing to
// clobber by default) on import. The composable adds the same
// "already exists" sniff used by usePDFCovers so callers can branch
// to a ConfirmDialog without parsing raw backend error strings.
type ExportArchiveOutcome =
  | { ok: true; zipPath: string; files: string[] }
  | { ok: false; message: string };

async function exportArchive(id: string, zipPath: string): Promise<ExportArchiveOutcome> {
  if (!id || !zipPath) {
    return { ok: false, message: "Plugin id and zip path required." };
  }
  try {
    const res = await PluginSvc.ExportArchive(id, zipPath);
    return {
      ok: true,
      zipPath: res?.zip_path ?? zipPath,
      files: res?.files ?? [],
    };
  } catch (err) {
    return { ok: false, message: String(err) };
  }
}

type ImportArchiveOutcome =
  | { ok: true; id: string; overwritten: boolean; files: string[] }
  | { ok: false; code: "exists" | "error"; message: string };

async function importArchive(zipPath: string, overwrite: boolean): Promise<ImportArchiveOutcome> {
  if (!zipPath) {
    return { ok: false, code: "error", message: "Zip path required." };
  }
  try {
    const res = await PluginSvc.ImportArchive(zipPath, overwrite);
    await refresh();
    return {
      ok: true,
      id: res?.id ?? "",
      overwritten: res?.overwritten ?? false,
      files: res?.files ?? [],
    };
  } catch (err) {
    const message = String(err);
    const code: "exists" | "error" = message.toLowerCase().includes("already exists") ? "exists" : "error";
    return { ok: false, code, message };
  }
}

const selectedPlugin = computed<ListResult | null>(() => {
  if (!selectedID.value) return null;
  return plugins.value.find((p) => p.id === selectedID.value) ?? null;
});

export function usePlugins() {
  if (!loaded) void refresh();
  if (!workspacesLoaded) void refreshWorkspaces();
  return {
    plugins,
    selectedID,
    selectedPlugin,
    workspaceIDs,
    refresh,
    create,
    remove,
    exportArchive,
    importArchive,
  };
}
