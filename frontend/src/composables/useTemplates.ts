import { computed, ref } from "vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const filenames = ref<string[]>([]);
const cache = ref<Map<string, Template | null>>(new Map());
const selectedFilename = ref<string>("");
let loaded = false;

async function refresh(): Promise<void> {
  filenames.value = await TemplateSvc.ListTemplates();
  loaded = true;
  // Eagerly load each into the cache so the sidebar can show display
  // names. Number of templates is small in practice; this avoids the
  // sidebar flickering as each row resolves its label.
  const next = new Map<string, Template | null>();
  for (const f of filenames.value) {
    next.set(f, await TemplateSvc.LoadTemplate(f));
  }
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
    cache,
    selectedFilename,
    selectedTemplate,
    refresh,
    load,
    create,
  };
}
