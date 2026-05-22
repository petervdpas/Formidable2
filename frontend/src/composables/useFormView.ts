import { computed, ref } from "vue";
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

  // ── load / refresh ────────────────────────────────────────────────
  async function open(templateName: string, datafile: string) {
    loading.value = true;
    error.value = "";
    try {
      const v = await FormSvc.BuildView(templateName, datafile);
      view.value = v;
      draft.value = clone(v);
    } catch (e) {
      error.value = String(e);
      view.value = null;
      draft.value = null;
    } finally {
      loading.value = false;
    }
  }

  function close() {
    view.value = null;
    draft.value = null;
    error.value = "";
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
    try {
      const payload = new SavePayload({
        datafile: draft.value.datafile,
        values: draft.value.values,
        meta: draft.value.meta,
      });
      const next = await FormSvc.SaveValues(draft.value.template.filename, payload);
      view.value = next;
      draft.value = clone(next);
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
