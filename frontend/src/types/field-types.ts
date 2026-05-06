// Field-type registry — single source of truth lives in Go
// (internal/modules/template/field_registry.go). This module loads
// the backend data on first call and merges it with the few
// frontend-only display concerns: i18n labelKey + per-type "default
// value when creating a new field".
//
// Public surface intentionally matches the previous static
// `FIELD_TYPES`: getFieldTypeDef / isRowHidden / selectableTypes /
// FIELD_TYPES — so existing consumers keep working without changes.
//
// Boot ordering: useFieldTypesLoader (in main.ts) kicks off load()
// before the app mounts, so by the time any component calls
// selectableTypes() the registry is populated. If the call beats the
// load (shouldn't happen in normal flow), the helpers degrade
// gracefully — the dropdown is empty and isRowHidden returns false.

import { ref, type Ref } from "vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

/** Stable IDs for every row the Edit Field modal can render. */
export type FieldEditRowId =
  | "key"
  | "type"
  | "format"
  | "summary_field"
  | "expression_item"
  | "two_column"
  | "collapsible"
  | "readonly"
  | "label"
  | "description"
  | "default"
  | "options"
  | "code_group"
  | "latex_group"
  | "api_group";

export interface FieldTypeDef {
  /** YAML/JSON `type` value (e.g. "text"). */
  id: string;
  /** Human label key for the Type dropdown. */
  labelKey: string;
  /** Default value for newly created fields of this type. */
  defaultValue?: () => unknown;
  /** Rows to HIDE in the Edit Field modal — derived from the
   *  backend's forbidden attributes for this type. */
  hiddenRows: FieldEditRowId[];
  /** True for marker types (looper, loopstart, loopstop) that
   *  don't carry a stored value. */
  metaOnly?: boolean;
}

// Frontend-only display concerns. Keys must align with backend type IDs.
const LABEL_KEYS: Record<string, string> = {
  text: "workspace.templates.field_type.text",
  boolean: "workspace.templates.field_type.boolean",
  dropdown: "workspace.templates.field_type.dropdown",
  multioption: "workspace.templates.field_type.multioption",
  radio: "workspace.templates.field_type.radio",
  textarea: "workspace.templates.field_type.textarea",
  number: "workspace.templates.field_type.number",
  range: "workspace.templates.field_type.range",
  date: "workspace.templates.field_type.date",
  list: "workspace.templates.field_type.list",
  table: "workspace.templates.field_type.table",
  image: "workspace.templates.field_type.image",
  link: "workspace.templates.field_type.link",
  tags: "workspace.templates.field_type.tags",
  latex: "workspace.templates.field_type.latex",
  code: "workspace.templates.field_type.code",
  api: "workspace.templates.field_type.api",
  guid: "workspace.templates.field_type.guid",
  looper: "workspace.templates.field_type.looper",
  loopstart: "workspace.templates.field_type.loopstart",
  loopstop: "workspace.templates.field_type.loopstop",
};

const DEFAULT_FACTORY: Record<string, () => unknown> = {
  text: () => "",
  boolean: () => false,
  dropdown: () => "",
  multioption: () => [],
  radio: () => "",
  textarea: () => "",
  number: () => 0,
  range: () => 50,
  date: () => "",
  list: () => [],
  table: () => [],
  image: () => "",
  link: () => ({ href: "", text: "" }),
  tags: () => [],
  latex: () => "",
  code: () => "",
  api: () => ({ id: "", overrides: {} }),
  guid: () => "",
};

// Map backend forbidden-attribute names to frontend row IDs. The
// backend uses bare group names ("code", "latex", "api"); the modal
// renders these as single rows named "<g>_group". Anything not in
// the map is assumed to be 1:1 with a row id (label, description,
// default, options, summary_field, expression_item, two_column,
// collapsible, readonly, format).
function attrToRow(attr: string): FieldEditRowId | null {
  switch (attr) {
    case "code":  return "code_group";
    case "latex": return "latex_group";
    case "api":   return "api_group";
    case "primary_key": return null; // no FE row for this
    default: return attr as FieldEditRowId;
  }
}

// Module-scope cache. Populated by load(); drives all the helper
// functions below. Reactive so any component reading `FIELD_TYPES`
// (or any helper that derives from it) sees the populated array as
// soon as the load resolves.
const registry: Ref<FieldTypeDef[]> = ref([]);
let loadPromise: Promise<void> | null = null;

async function load(): Promise<void> {
  const defs = (await TemplateSvc.FieldTypes()) ?? [];
  registry.value = defs.map((d) => ({
    id: d.id,
    labelKey: LABEL_KEYS[d.id] ?? d.id,
    defaultValue: DEFAULT_FACTORY[d.id],
    metaOnly: d.meta_only,
    hiddenRows: (d.forbidden_attributes ?? [])
      .map(attrToRow)
      .filter((r): r is FieldEditRowId => r !== null),
  }));
}

/** Kicked off by main.ts before mount so components see a populated
 *  registry on first render. Idempotent. */
export function ensureFieldTypesLoaded(): Promise<void> {
  if (!loadPromise) loadPromise = load();
  return loadPromise;
}

/** Reactive registry array — same shape as the old FIELD_TYPES
 *  constant. Consumers that need to iterate (e.g. dropdown options)
 *  read this; it'll re-render once load() completes. */
export const FIELD_TYPES = registry;

export function getFieldTypeDef(id: string): FieldTypeDef | undefined {
  // Reads through the ref so callers inside computed/templates
  // re-run when load() resolves.
  return registry.value.find((d) => d.id === id);
}

export function isRowHidden(typeId: string, rowId: FieldEditRowId): boolean {
  const def = getFieldTypeDef(typeId);
  if (!def) return false;
  return def.hiddenRows.includes(rowId);
}

/** Type IDs eligible to appear in the Edit modal's Type dropdown.
 *
 *  - For new fields (isNew): include `looper` (synthesizes a
 *    loopstart/loopstop pair on confirm); hide loopstart/loopstop.
 *  - For existing loopstart/loopstop: lock to that same type — the
 *    user can't "convert" half of a pair into something else.
 *  - For other existing fields: hide `looper`, `loopstart`, `loopstop`. */
export function selectableTypes(currentType: string, isNew = false): FieldTypeDef[] {
  return registry.value.filter((t) => {
    if (t.id === "loopstart" || t.id === "loopstop") {
      return t.id === currentType;
    }
    if (t.id === "looper") {
      return isNew;
    }
    return true;
  });
}
