import { computed, ref, watch } from "vue";
import {
  Service as FormSvc,
  SavePayload,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import type { FormView } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import { FormMeta } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";

// useFormView holds the current (template, datafile) view and its
// reactive working draft. Vue components bind directly to draft.values
// for two-way editing; dirty diffs against the freshly-loaded view.
//
// One instance per StorageWorkspace mount. Module-scope singleton is
// not needed here - unlike templates, only one form is editable at a
// time inside one workspace, and the state is owned by that workspace.

export function useFormView() {
  const view = ref<FormView | null>(null);
  const draft = ref<FormView | null>(null);
  const loading = ref(false);
  const error = ref<string>("");

  // ── undo / redo ───────────────────────────────────────────────────
  // Edit history for the working draft. Components mutate draft.values /
  // draft.meta directly via v-model, so there's no single commit point
  // to hook; instead a debounced deep watch records snapshots. A burst
  // of keystrokes (or a lazy-committed number field) collapses into one
  // undo step. Snapshots cover exactly what `dirty` tracks: values +
  // meta (loop edits live under values[key], so they're included).
  const UNDO_CAP = 200;
  const RECORD_DELAY_MS = 400;
  const undoStack = ref<string[]>([]);
  const redoStack = ref<string[]>([]);
  let baseline = "";
  let recordTimer: ReturnType<typeof setTimeout> | null = null;

  const canUndo = computed(() => undoStack.value.length > 0);
  const canRedo = computed(() => redoStack.value.length > 0);

  function snap(): string {
    return JSON.stringify({ values: draft.value?.values, meta: draft.value?.meta });
  }

  // Commit any pending edit burst as one undo step. Called by the
  // debounce timer and synchronously before an undo so in-progress
  // edits become their own reversible step.
  function flushRecord() {
    if (recordTimer) {
      clearTimeout(recordTimer);
      recordTimer = null;
    }
    if (!draft.value) return;
    const cur = snap();
    if (cur === baseline) return;
    undoStack.value.push(baseline);
    if (undoStack.value.length > UNDO_CAP) undoStack.value.shift();
    redoStack.value = [];
    baseline = cur;
  }

  function scheduleRecord() {
    if (recordTimer) clearTimeout(recordTimer);
    recordTimer = setTimeout(flushRecord, RECORD_DELAY_MS);
  }

  // Apply a snapshot to the draft and rebaseline synchronously, so the
  // deep watch the mutation triggers finds no diff and records nothing.
  function applySnap(s: string) {
    if (!draft.value) return;
    const p = JSON.parse(s) as { values: FormView["values"]; meta: FormView["meta"] };
    draft.value.values = p.values;
    draft.value.meta = p.meta;
    baseline = s;
  }

  function resetHistory() {
    if (recordTimer) {
      clearTimeout(recordTimer);
      recordTimer = null;
    }
    undoStack.value = [];
    redoStack.value = [];
    baseline = snap();
  }

  function undo() {
    flushRecord();
    if (!canUndo.value) return;
    redoStack.value.push(snap());
    applySnap(undoStack.value.pop()!);
  }

  function redo() {
    if (!canRedo.value) return;
    undoStack.value.push(snap());
    applySnap(redoStack.value.pop()!);
  }

  watch(draft, scheduleRecord, { deep: true });

  // ── load / refresh ────────────────────────────────────────────────
  async function open(templateName: string, datafile: string) {
    loading.value = true;
    error.value = "";
    try {
      const v = await FormSvc.BuildView(templateName, datafile);
      view.value = v;
      draft.value = clone(v);
      resetHistory();
    } catch (e) {
      error.value = String(e);
      view.value = null;
      draft.value = null;
      resetHistory();
    } finally {
      loading.value = false;
    }
  }

  function close() {
    view.value = null;
    draft.value = null;
    error.value = "";
    resetHistory();
  }

  // ── dirty tracking ────────────────────────────────────────────────
  // Deep JSON-compare against the loaded view. Cheap and correct for
  // the JSON-serializable shape Vue holds; if perf matters later we
  // can swap for structural compare.
  const dirty = computed<boolean>(() => {
    if (!draft.value || !view.value) return false;
    return (
      JSON.stringify(draft.value.values) !== JSON.stringify(view.value.values) ||
      JSON.stringify(draft.value.meta) !== JSON.stringify(view.value.meta)
    );
  });

  // ── save / reset ──────────────────────────────────────────────────
  async function save(): Promise<{ ok: boolean; message?: string }> {
    if (!draft.value) return { ok: false, message: "no draft" };
    if (!draft.value.template?.filename) {
      return { ok: false, message: "template missing" };
    }
    if (!draft.value.datafile) {
      return { ok: false, message: "datafile missing" };
    }
    // Capture any in-progress edit burst as an undo step before the
    // draft is replaced, so undo still reaches the pre-save state.
    flushRecord();
    try {
      const payload = new SavePayload({
        datafile: draft.value.datafile,
        values: draft.value.values,
        meta: draft.value.meta,
      });
      const next = await FormSvc.SaveValues(draft.value.template.filename, payload);
      view.value = next;
      draft.value = clone(next);
      // Rebaseline to the saved shape but keep the stacks: undo after a
      // save reverts to the pre-save edits (re-marking the form dirty).
      baseline = snap();
      return { ok: true };
    } catch (e) {
      return { ok: false, message: String(e) };
    }
  }

  function reset() {
    if (view.value) draft.value = clone(view.value);
  }

  // ── delete ────────────────────────────────────────────────────────
  async function remove(): Promise<{ ok: boolean; message?: string }> {
    if (!view.value?.template?.filename || !view.value?.datafile) {
      return { ok: false, message: "nothing to delete" };
    }
    try {
      await FormSvc.DeleteForm(view.value.template.filename, view.value.datafile);
      close();
      return { ok: true };
    } catch (e) {
      return { ok: false, message: String(e) };
    }
  }

  return {
    view,
    draft,
    loading,
    error,
    dirty,
    canUndo,
    canRedo,
    undo,
    redo,
    open,
    close,
    save,
    reset,
    remove,
  };
}

// JSON round-trip clone. Works for our pure-data shape (no functions,
// no Date objects, no class instances beyond what Wails generated).
function clone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v));
}

// Re-export so workspaces can name the type without a deep import.
export type { FormView, FormMeta };
