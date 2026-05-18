import { computed, ref } from "vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as ConfigSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";

const filenames = ref<string[]>([]);
// enabledFilenames is the use-side picker list — the subset of
// `filenames` allowed by the active profile's EnabledTemplates list.
// Empty EnabledTemplates → equal to `filenames` (the "all enabled"
// default). Filtering happens server-side via ConfigSvc.ListEnabledTemplates
// so there's one source of truth for "what's pickable" — per the
// backend-owns-data rule, no JS intersection.
const enabledFilenames = ref<string[]>([]);
const cache = ref<Map<string, Template | null>>(new Map());
const selectedFilename = ref<string>("");
let loaded = false;

// After a remote-sourced filesystem change (gigot/git pull, clone,
// reclone) the cached template list goes stale. The pull workspace
// dispatches `formidable:context-reloaded` once it's done; we re-read
// from disk so the sidebar reflects deletions/additions without an
// app restart.
if (typeof window !== "undefined") {
  window.addEventListener("formidable:context-reloaded", () => {
    if (loaded) void refresh();
  });
}

async function refresh(): Promise<void> {
  filenames.value = await TemplateSvc.ListTemplates();
  loaded = true;
  // One IPC call for the whole list — backend's per-name cache + the
  // batched LoadMany endpoint mean a saved-or-cold list of N templates
  // resolves in a single round-trip. Per-row errors land in `error`
  // and keep the rest of the batch usable.
  const results = await TemplateSvc.LoadMany(filenames.value);
  const next = new Map<string, Template | null>();
  for (const r of results) {
    next.set(r.filename, r.template ?? null);
  }
  cache.value = next;
  // Refresh the picker subset alongside — keeps the two refs in sync
  // so consumers don't see a window where filenames was updated but
  // enabledFilenames still reflects a stale corpus.
  await refreshEnabled();
}

// refreshEnabled re-fetches the use-side subset from the backend. Call
// after toggling enabled_templates in Settings → Templates, after a
// remote pull, or anywhere the live folder may have changed. Tolerant
// of backend failure: keeps the previous subset so the picker doesn't
// suddenly empty on a transient IPC error.
async function refreshEnabled(): Promise<void> {
  try {
    enabledFilenames.value = (await ConfigSvc.ListEnabledTemplates()) ?? [];
  } catch {
    // Defensive: don't wipe the subset on a transient failure.
  }
}

async function refreshOne(filename: string): Promise<void> {
  if (!filename) return;
  const t = await TemplateSvc.LoadTemplate(filename);
  const next = new Map(cache.value);
  next.set(filename, t);
  cache.value = next;
}

async function load(filename: string): Promise<Template | null> {
  if (cache.value.has(filename)) return cache.value.get(filename) ?? null;
  const t = await TemplateSvc.LoadTemplate(filename);
  cache.value.set(filename, t);
  return t;
}

async function create(filename: string): Promise<{ ok: boolean; code?: string; message?: string }> {
  await ensureLoaded();
  if (filenames.value.includes(filename)) {
    return { ok: false, code: "exists" };
  }
  const stub = new Template({
    name: filename.replace(/\.yaml$/, ""),
    filename,
    fields: [],
  });
  try {
    await TemplateSvc.SaveTemplate(filename, stub);
    await refresh();
    selectedFilename.value = filename;
    return { ok: true };
  } catch (err) {
    return { ok: false, message: String(err) };
  }
}

async function remove(filename: string): Promise<{ ok: boolean; message?: string }> {
  try {
    await TemplateSvc.DeleteTemplate(filename);
    if (selectedFilename.value === filename) selectedFilename.value = "";
    // Splice the entry out in place — mirrors the StorageWorkspace
    // delete pattern, so the rest of the sidebar list (and its scroll
    // position) stays untouched. cache.value.delete keeps the lookup
    // map consistent without forcing a full TemplateSvc.LoadTemplate
    // pass across every other template.
    const idx = filenames.value.indexOf(filename);
    if (idx >= 0) filenames.value.splice(idx, 1);
    cache.value.delete(filename);
    return { ok: true };
  } catch (err) {
    return { ok: false, message: String(err) };
  }
}

async function ensureLoaded(): Promise<void> {
  if (!loaded) await refresh();
}

const FILENAME_RE = /^[a-z0-9-]+\.yaml$/;
export function isValidTemplateFilename(name: string): boolean {
  return FILENAME_RE.test(name);
}

const selectedTemplate = computed<Template | null>(() => {
  if (!selectedFilename.value) return null;
  return cache.value.get(selectedFilename.value) ?? null;
});

export function useTemplates() {
  if (!loaded) refresh();
  return {
    filenames,
    enabledFilenames,
    cache,
    selectedFilename,
    selectedTemplate,
    refresh,
    refreshEnabled,
    refreshOne,
    load,
    create,
    remove,
  };
}
