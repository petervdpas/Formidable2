// Field-type registry. Mirrors `utils/fieldTypes.js` from the original
// Formidable: each entry declares which rows in the Edit Field modal
// are HIDDEN for that type. The modal renders the union of all rows;
// per-type visibility is data-driven from `hiddenRows` so adding a new
// type is one entry, no component changes.

/** Stable IDs for every row the Edit Field modal can render. */
export type FieldEditRowId =
  | "key"
  | "type"
  | "format"               // textarea-only — markdown vs plain
  | "summary_field"        // loopstart-only — pick a child key as label
  | "expression_item"
  | "two_column"
  | "collapsible"
  | "readonly"
  | "label"
  | "description"
  | "default"
  | "options"
  | "code_group"           // run_mode / allow_run / hide_field / input_mode / api_mode / api_pick
  | "latex_group"          // use_fenced / rows
  | "api_group";           // collection / use_picker / id / allowed_ids / map

export interface FieldTypeDef {
  /** YAML/JSON `type` value (e.g. "text"). */
  id: string;
  /** Human label used in the Type dropdown — i18n key is preferred but
   *  fallback to this when no translation exists. */
  labelKey: string;
  /** Default for newly created fields of this type. */
  defaultValue?: () => unknown;
  /** Rows to HIDE in the Edit Field modal. */
  hiddenRows: FieldEditRowId[];
  /** True when this type is a marker (no input value of its own). */
  metaOnly?: boolean;
}

/** Code-specific rows are toggled as a group, not individually. */
const HIDE_CODE: FieldEditRowId[] = ["code_group"];
const HIDE_LATEX: FieldEditRowId[] = ["latex_group"];
const HIDE_API: FieldEditRowId[] = ["api_group"];

export const FIELD_TYPES: FieldTypeDef[] = [
  {
    id: "guid",
    labelKey: "workspace.templates.field_type.guid",
    defaultValue: () => "",
    hiddenRows: [
      "label",
      "description",
      "default",
      "options",
      "summary_field",
      "expression_item",
      "two_column",
      "collapsible",
      "readonly",
      "format",
      ...HIDE_CODE,
      ...HIDE_LATEX,
      ...HIDE_API,
    ],
  },
  {
    id: "text",
    labelKey: "workspace.templates.field_type.text",
    defaultValue: () => "",
    hiddenRows: ["summary_field", "collapsible", "format", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "boolean",
    labelKey: "workspace.templates.field_type.boolean",
    defaultValue: () => false,
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "options", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "dropdown",
    labelKey: "workspace.templates.field_type.dropdown",
    defaultValue: () => "",
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "multioption",
    labelKey: "workspace.templates.field_type.multioption",
    defaultValue: () => [],
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "radio",
    labelKey: "workspace.templates.field_type.radio",
    defaultValue: () => "",
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "textarea",
    labelKey: "workspace.templates.field_type.textarea",
    defaultValue: () => "",
    hiddenRows: ["summary_field", "collapsible", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "latex",
    labelKey: "workspace.templates.field_type.latex",
    defaultValue: () => "",
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "expression_item", "two_column", "options", ...HIDE_CODE, ...HIDE_API],
  },
  {
    id: "number",
    labelKey: "workspace.templates.field_type.number",
    defaultValue: () => 0,
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "options", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "range",
    labelKey: "workspace.templates.field_type.range",
    defaultValue: () => 50,
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "options", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "date",
    labelKey: "workspace.templates.field_type.date",
    defaultValue: () => "",
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "options", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "list",
    labelKey: "workspace.templates.field_type.list",
    defaultValue: () => [],
    hiddenRows: ["summary_field", "readonly", "format", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "table",
    labelKey: "workspace.templates.field_type.table",
    defaultValue: () => [],
    hiddenRows: ["summary_field", "readonly", "format", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "image",
    labelKey: "workspace.templates.field_type.image",
    defaultValue: () => "",
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "options", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "link",
    labelKey: "workspace.templates.field_type.link",
    defaultValue: () => ({ href: "", text: "" }),
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "options", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "tags",
    labelKey: "workspace.templates.field_type.tags",
    defaultValue: () => [],
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "options", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "code",
    labelKey: "workspace.templates.field_type.code",
    defaultValue: () => "",
    hiddenRows: ["summary_field", "collapsible", "readonly", "format", "expression_item", "two_column", ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "api",
    labelKey: "workspace.templates.field_type.api",
    defaultValue: () => ({ id: "", overrides: {} }),
    hiddenRows: ["summary_field", "default", "options", "format", "expression_item", "two_column", "collapsible", "readonly", ...HIDE_CODE, ...HIDE_LATEX],
  },
  // Loop pair — kept around so the type dropdown can echo "Loop Start"
  // for an existing loopstart row, but they're not user-selectable
  // (looper synthesis creates them as a pair).
  {
    id: "loopstart",
    labelKey: "workspace.templates.field_type.loopstart",
    metaOnly: true,
    hiddenRows: ["default", "options", "expression_item", "two_column", "collapsible", "readonly", "format", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
  {
    id: "loopstop",
    labelKey: "workspace.templates.field_type.loopstop",
    metaOnly: true,
    hiddenRows: ["description", "default", "options", "expression_item", "two_column", "collapsible", "readonly", "format", "summary_field", ...HIDE_CODE, ...HIDE_LATEX, ...HIDE_API],
  },
];

const BY_ID = new Map(FIELD_TYPES.map((t) => [t.id, t]));

export function getFieldTypeDef(id: string): FieldTypeDef | undefined {
  return BY_ID.get(id);
}

export function isRowHidden(typeId: string, rowId: FieldEditRowId): boolean {
  const def = BY_ID.get(typeId);
  if (!def) return false;
  return def.hiddenRows.includes(rowId);
}

/** Type IDs eligible to appear in the Edit modal's Type dropdown. We
 *  hide loopstart/loopstop unless the current field already has that
 *  type — those come from the synthetic "looper" creation, not from
 *  changing an existing field's type. */
export function selectableTypes(currentType: string): FieldTypeDef[] {
  return FIELD_TYPES.filter((t) => {
    const isLoop = t.id === "loopstart" || t.id === "loopstop";
    if (isLoop) return t.id === currentType;
    return true;
  });
}
