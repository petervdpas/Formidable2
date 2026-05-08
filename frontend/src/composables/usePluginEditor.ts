import { computed, ref, watch } from "vue";
import {
  Service as PluginSvc,
  Manifest,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { usePlugins } from "./usePlugins";

// Module-scope singleton, mirroring useTemplateEditor: at most one
// plugin is being edited at a time and the workspace + topbar menu
// share the same draft / dirty state.
const draftManifest = ref<Manifest | null>(null);
const draftSource = ref<string>("");

// Baseline is what we last loaded from disk for the current
// selection — dirty compares against it. We deliberately clone the
// manifest into a fresh object so mutating draftManifest never
// mutates the cached list result.
let baselineManifest: Manifest | null = null;
let baselineSource = "";

const { selectedPlugin, selectedID, refresh } = usePlugins();

function clone<T>(v: T): T {
  // JSON round-trip — Manifest is a plain shape; the binding
  // constructor accepts Partial<Manifest>. Same trick
  // useTemplateEditor uses for Template.
  return JSON.parse(JSON.stringify(v));
}

function deepEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

const dirty = computed<boolean>(() => {
  if (!draftManifest.value || !baselineManifest) return false;
  if (!deepEqual(draftManifest.value, baselineManifest)) return true;
  if (draftSource.value !== baselineSource) return true;
  return false;
});

// Whenever the sidebar selection changes (or the cached list
// refreshes after a save) reload the baseline + draft from the
// backend. GetSource is a separate call because the list endpoint
// only returns parsed manifests — keeps the list payload small.
watch(
  selectedPlugin,
  async (p) => {
    if (!p) {
      draftManifest.value = null;
      draftSource.value = "";
      baselineManifest = null;
      baselineSource = "";
      return;
    }
    const mf = clone(p.manifest);
    let src = "";
    try {
      src = await PluginSvc.GetSource(p.id);
    } catch {
      src = "";
    }
    baselineManifest = clone(mf);
    baselineSource = src;
    draftManifest.value = mf;
    draftSource.value = src;
  },
  { immediate: true },
);

export type SaveOutcome =
  | { ok: true }
  | { ok: false; reason: "no-draft" }
  | { ok: false; reason: "exception"; message: string };

async function save(): Promise<SaveOutcome> {
  if (!draftManifest.value || !selectedID.value) {
    return { ok: false, reason: "no-draft" };
  }
  try {
    const payload = new Manifest(clone(draftManifest.value));
    await PluginSvc.Save(selectedID.value, payload, draftSource.value);
    // Pull canonical state back so the sidebar and caches reflect
    // the new manifest immediately.
    await refresh();
    baselineManifest = clone(payload);
    baselineSource = draftSource.value;
    return { ok: true };
  } catch (err) {
    return { ok: false, reason: "exception", message: String(err) };
  }
}

function reset(): void {
  if (!baselineManifest) {
    draftManifest.value = null;
    draftSource.value = "";
    return;
  }
  draftManifest.value = clone(baselineManifest);
  draftSource.value = baselineSource;
}

export function usePluginEditor() {
  return {
    draftManifest,
    draftSource,
    dirty,
    save,
    reset,
  };
}
