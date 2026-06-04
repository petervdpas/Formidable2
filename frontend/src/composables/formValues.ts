import type { InjectionKey, Ref } from "vue";

/*
 * FormValuesContext is the bridge that lets an inline virtual field read sibling
 * data values from the active draft and (for live formula fields) trigger a
 * compute. The workspace owning the open form (StorageWorkspace) provides it;
 * a virtual field renderer that projects another field's value injects it.
 *
 * Live compute reads the SAVED record on the backend, so it is blocked while the
 * form is dirty or unsaved: the consumer disables the button, and `compute`
 * no-ops as a backstop.
 *
 * Workspaces rendering form rows in isolation (e.g. PluginsWorkspace) don't
 * provide it; the consumer then falls back to an inert hint.
 */
export interface FormValuesContext {
  values: Ref<Record<string, unknown>>;
  /** True while the open form has unsaved edits. */
  dirty: Ref<boolean>;
  /** True when the form is backed by a saved record (not a fresh draft). */
  saved: Ref<boolean>;
  /** Map of target field key -> the live formula field key that writes into it.
   *  Drives the Compute button rendered beneath the target field. A formula
   *  field is otherwise invisible in the rendered form. */
  liveFormulaTargets: Ref<Record<string, string>>;
  /** Compute a live formula field by its key. The backend resolves the bound
   *  formula + target and returns the value; the workspace writes it in. */
  compute: (fieldKey: string) => Promise<void>;
}

export const FORM_VALUES_KEY: InjectionKey<FormValuesContext> =
  Symbol("formValues");
