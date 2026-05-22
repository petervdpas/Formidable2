import { computed, ref, watch } from "vue";
import {
  Service as PluginSvc,
  Manifest,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Widget } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";
import { Kind as WidgetKind } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";
import { usePlugins } from "./usePlugins";

// Module-scope singleton, mirroring useTemplateEditor: at most one
// plugin is being edited at a time and the workspace + topbar menu
// share the same draft / dirty state.
//
// draftForm holds the parsed form.json - a heterogeneous, ordered
// list where each entry is EITHER a template Field (input) or a
// formwidget.Widget (live display slot). Position in the array IS
// the render order; the form editor's drag-drop list operates on
// the same array, so dropping a widget between two fields persists
// as the same array order on disk.
const draftManifest = ref<Manifest | null>(null);
const draftSource = ref<string>("");
const draftForm = ref<Array<Field | Widget>>([]);

// Baseline is what we last loaded from disk for the current
// selection - dirty compares against it.
let baselineManifest: Manifest | null = null;
let baselineSource = "";
let baselineForm: Array<Field | Widget> = [];

const { selectedPlugin, selectedID, refresh } = usePlugins();

function clone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v));
}

function deepEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

// isWidget routes JSON entries to the right renderer: Widgets have
// a `kind` in the closed widget enum, Fields have a `type` in the
// template field-type set. The two namespaces don't overlap, so
// checking `kind` is a safe discriminator without an explicit
// `_kind` field.
export function isWidget(entry: Field | Widget): entry is Widget {
  const k = (entry as Widget).kind;
  return (
    k === WidgetKind.KindProgressBar || k === WidgetKind.KindStatusMessage
  );
}

const dirty = computed<boolean>(() => {
  if (!draftManifest.value || !baselineManifest) return false;
  if (!deepEqual(draftManifest.value, baselineManifest)) return true;
  if (draftSource.value !== baselineSource) return true;
  if (!deepEqual(draftForm.value, baselineForm)) return true;
  return false;
});

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
    let entries: Array<Field | Widget> = [];
    try {
      const raw = await PluginSvc.GetForm(p.id);
      const parsed = JSON.parse(raw || "[]");
      if (Array.isArray(parsed)) {
        entries = parsed;
      } else if (
        parsed &&
        typeof parsed === "object" &&
        Array.isArray((parsed as { fields?: unknown }).fields)
      ) {
        entries = (parsed as { fields: Array<Field | Widget> }).fields;
      }
    } catch {
      entries = [];
    }
    baselineManifest = clone(mf);
    baselineSource = src;
    baselineForm = clone(entries);
    draftManifest.value = mf;
    draftSource.value = src;
    draftForm.value = entries;
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
