import type { ColumnDef, OptionRow } from "../components/fields/OptionsEditor.vue";

// Per-field-type column presets for the OptionsEditor. Mirrors
// `utils/optionsEditor.js` from the original Formidable.
//
// Types not in `SUPPORTED_OPTION_TYPES` get a "Options not available"
// message in the modal (OptionsEditor isn't rendered).

export const SUPPORTED_OPTION_TYPES = new Set([
  "boolean",
  "dropdown",
  "multioption",
  "radio",
  "range",
  "list",
  "table",
  // file-path uses options to declare allowed extension filters
  // ("*.json", "*.md") that drive the native picker's filter dropdown.
  "file-path",
]);

const DEFAULT_COLUMNS: ColumnDef[] = [
  { key: "value", type: "text", placeholder: "value" },
  { key: "label", type: "text", placeholder: "label" },
];

const LIST_COLUMNS: ColumnDef[] = [
  {
    key: "type",
    type: "dropdown",
    options: ["fixed", "custom"],
    defaultValue: "fixed",
    placeholder: "type",
    onChange(value, row): OptionRow {
      if (value === "custom") {
        return { ...row, value: "[[custom]]", _valueLocked: true };
      }
      // Switching back from custom — unlock and clear the placeholder.
      const cleared = row["value"] === "[[custom]]" ? "" : row["value"];
      return { ...row, value: cleared, _valueLocked: false };
    },
  },
  { key: "value", type: "text", placeholder: "value" },
  { key: "label", type: "text", placeholder: "label" },
];

// Subrows (choices / reference) for table are deferred — basic table
// preset just shows key / type / label.
const TABLE_COLUMNS: ColumnDef[] = [
  { key: "value", type: "text", placeholder: "key" },
  {
    key: "type",
    type: "dropdown",
    options: ["string", "number", "date", "bool", "dropdown", "reference"],
    defaultValue: "string",
    placeholder: "type",
  },
  { key: "label", type: "text", placeholder: "label" },
];

// file-path: each row is one entry in the picker's filter dropdown.
// `pattern` is the platform-native glob ("*.json", "*.md;*.markdown");
// `label` is the human-readable name shown above the pattern.
const FILE_PATH_COLUMNS: ColumnDef[] = [
  { key: "label", type: "text", placeholder: "JSON" },
  { key: "pattern", type: "text", placeholder: "*.json" },
];

const PRESETS: Record<string, ColumnDef[]> = {
  list: LIST_COLUMNS,
  table: TABLE_COLUMNS,
  "file-path": FILE_PATH_COLUMNS,
};

export function columnsFor(typeId: string): ColumnDef[] | null {
  if (!SUPPORTED_OPTION_TYPES.has(typeId)) return null;
  return PRESETS[typeId] ?? DEFAULT_COLUMNS;
}
