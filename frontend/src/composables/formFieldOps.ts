import type { InjectionKey } from "vue";

/*
 * FormFieldOps is the bridge between the workspace that owns the open form
 * (StorageWorkspace knows the template + datafile) and the list/table field
 * widgets rendered deep inside the form. A widget injects this context and
 * calls sortField / dedupField with only its own field key. The workspace
 * sends the pointer (template, datafile, field) to the backend, which fetches
 * that field from the saved record, sorts/dedups it, and returns the new value.
 * The widget applies the value via update:modelValue; the normal Save persists
 * it. The data work happens on the Go side, never in the browser, and the
 * sort/dedup itself does not write to disk.
 *
 * Each call resolves to the new field value, or undefined when the op could
 * not run (no open record, or a backend error already surfaced as a toast).
 *
 * Workspaces that render fields in isolation (e.g. PluginsWorkspace) don't
 * provide this context; the widgets then hide their sort/dedup buttons.
 */
export interface FormFieldOps {
  sortField: (
    fieldKey: string,
    opts?: { column?: string; direction?: "asc" | "desc" },
  ) => Promise<unknown | undefined>;
  dedupField: (
    fieldKey: string,
    opts?: { column?: string },
  ) => Promise<unknown | undefined>;
  /** Tables of the open template whose rows can be copied into table
   *  `fieldKey` (backend-decided: other top-level tables sharing a column). */
  tableCopySources: (fieldKey: string) => Promise<{ key: string; label: string }[]>;
  /** The table's saved rows plus the rows of table `sourceKey`, mapped by
   *  column key; rows already present are not added. `unknownReferences`
   *  lists codes copied into a reference column that match no loop item. */
  copyTable: (
    fieldKey: string,
    sourceKey: string,
  ) => Promise<{ value: unknown; added: number; unknownReferences: string[] } | undefined>;
}

// Nullable so a synthetic context (e.g. a slide list block, which isn't a saved
// record field) can provide null to suppress the backend sort/dedup ops.
export const FORM_FIELD_OPS_KEY: InjectionKey<FormFieldOps | null> = Symbol("formFieldOps");
