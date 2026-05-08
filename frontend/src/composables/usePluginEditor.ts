import { computed, ref, watch } from "vue";
import {
  Service as PluginSvc,
  Manifest,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { usePlugins } from "./usePlugins";

// Module-scope singleton, mirroring useTemplateEditor: at most one
// plugin is being edited at a time and the workspace + topbar menu
// share the same draft / dirty state.
const draftManifest = ref<Manifest | null>(null);
const draftSource = ref<string>("");
// draftForm is the parsed form.json shape — an array of template
// Fields, the same schema FormFieldRenderer + FieldEditModal use.
// Backend treats form.json as opaque text; we parse/serialize here.
const draftForm = ref<Field[]>([]);

// Baseline is what we last loaded from disk for the current
// selection — dirty compares against it. We deliberately clone the
// manifest into a fresh object so mutating draftManifest never
// mutates the cached list result.
let baselineManifest: Manifest | null = null;
let baselineSource = "";
let baselineForm: Field[] = [];

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
  if (!deepEqual(draftForm.value, baselineForm)) return true;
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
      draftForm.value = [];
      baselineManifest = null;
      baselineSource = "";
      baselineForm = [];
      return;
    }
    const mf = clone(p.manifest);
    let src = "";
    try {
      src = await PluginSvc.GetSource(p.id);
    } catch {
      src = "";
    }
    let formFields: Field[] = [];
    try {
      const raw = await PluginSvc.GetForm(p.id);
      const parsed = JSON.parse(raw || "[]");
      // Tolerate both shapes so an old experimental object form
      // ({fields:[...]}) doesn't silently drop the field list on
      // load. Canonical write is still a bare array.
      if (Array.isArray(parsed)) {
        formFields = parsed;
      } else if (
        parsed &&
        typeof parsed === "object" &&
        Array.isArray((parsed as { fields?: unknown }).fields)
      ) {
        formFields = (parsed as { fields: Field[] }).fields;
      }
    } catch {
      formFields = [];
    }
    baselineManifest = clone(mf);
    baselineSource = src;
    baselineForm = clone(formFields);
    draftManifest.value = mf;
    draftSource.value = src;
    draftForm.value = formFields;
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
    // Only ship form.json when the field list actually changed.
    // Backend treats empty string as "leave untouched" — so mode
    // toggles / description edits / source edits never reach the
    // form file. Hard belt-and-suspenders against ever wiping
    // user-authored form data via an unrelated save.
    const formChanged = !deepEqual(draftForm.value, baselineForm);
    const formJSON = formChanged
      ? JSON.stringify(draftForm.value, null, 2) + "\n"
      : "";
    await PluginSvc.Save(
      selectedID.value,
      payload,
      draftSource.value,
      formJSON,
    );
    // Pull canonical state back so the sidebar and caches reflect
    // the new manifest immediately.
    await refresh();
    baselineManifest = clone(payload);
    baselineSource = draftSource.value;
    baselineForm = clone(draftForm.value);
    return { ok: true };
  } catch (err) {
    return { ok: false, reason: "exception", message: String(err) };
  }
}

function reset(): void {
  if (!baselineManifest) {
    draftManifest.value = null;
    draftSource.value = "";
    draftForm.value = [];
    return;
  }
  draftManifest.value = clone(baselineManifest);
  draftSource.value = baselineSource;
  draftForm.value = clone(baselineForm);
}

export function usePluginEditor() {
  return {
    draftManifest,
    draftSource,
    draftForm,
    dirty,
    save,
    reset,
  };
}
