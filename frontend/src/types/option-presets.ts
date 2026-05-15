import type { ColumnDef, OptionRow } from "../components/fields/OptionsEditor.vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Per-field-type column presets for the OptionsEditor. Mirrors
// `utils/optionsEditor.js` from the original Formidable.
//
// Backend ownership: list/table column-type vocabularies live on the
// Go side (TemplateSvc.ListItemTypes / TemplateSvc.TableColumnTypes
// — see internal/modules/template/option_presets.go). This module
// loads them at boot (ensureOptionPresetsLoaded, kicked from main.ts)
// and caches the result. Same pattern as `field-types.ts`. The
// initial bootstrap values below match what the backend ships so the
// first render never sees an empty dropdown if the load is mid-flight.
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

// Bootstrap fallbacks. Used until ensureOptionPresetsLoaded resolves
// (or if it fails — degraded but functional). Match the canonical
// builtinTableColumnTypes / builtinListItemTypes in Go.
let _tableColumnTypes: string[] = ["string", "number", "date", "bool", "dropdown", "reference"];
let _listItemTypes: string[] = ["fixed", "custom"];

let loadPromise: Promise<void> | null = null;

async function load(): Promise<void> {
  try {
    const [tcols, ltypes] = await Promise.all([
      TemplateSvc.TableColumnTypes(),
      TemplateSvc.ListItemTypes(),
    ]);
    if (tcols && tcols.length > 0) {
      _tableColumnTypes = tcols.map((d) => d.name);
    }
    if (ltypes && ltypes.length > 0) {
      _listItemTypes = ltypes.map((d) => d.name);
    }
  } catch {
    // Stay on the bootstrap fallbacks — better empty UX than crash.
  }
}

export function ensureOptionPresetsLoaded(): Promise<void> {
  if (!loadPromise) loadPromise = load();
  return loadPromise;
}

const DEFAULT_COLUMNS: ColumnDef[] = [
  { key: "value", type: "text", placeholder: "value" },
  { key: "label", type: "text", placeholder: "label" },
];

function listColumns(): ColumnDef[] {
  return [
    {
      key: "type",
      type: "dropdown",
      options: [..._listItemTypes],
      defaultValue: _listItemTypes[0] ?? "fixed",
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
}

// Subrows (choices / reference) for table are deferred on both sides.
// "reference" cells are plain strings at the data layer; the renderer
// is supposed to string-compare each value against in-scope looper
// codes and emit an HTML anchor on match (TOC-style). A picker
// populated from looper codes is a convenience layer on top, not a
// constraint — free-form typing must still work. See
// internal/modules/template/option_presets.go for the Go-side note.
function tableColumns(): ColumnDef[] {
  return [
    { key: "value", type: "text", placeholder: "key" },
    {
      key: "type",
      type: "dropdown",
      options: [..._tableColumnTypes],
      defaultValue: _tableColumnTypes[0] ?? "string",
      placeholder: "type",
    },
    { key: "label", type: "text", placeholder: "label" },
  ];
}

// file-path: each row is one entry in the picker's filter dropdown.
// `pattern` is the platform-native glob ("*.json", "*.md;*.markdown");
// `label` is the human-readable name shown above the pattern.
const FILE_PATH_COLUMNS: ColumnDef[] = [
  { key: "label", type: "text", placeholder: "JSON" },
  { key: "pattern", type: "text", placeholder: "*.json" },
];

export function columnsFor(typeId: string): ColumnDef[] | null {
  if (!SUPPORTED_OPTION_TYPES.has(typeId)) return null;
  switch (typeId) {
    case "list":
      return listColumns();
    case "table":
      return tableColumns();
    case "file-path":
      return FILE_PATH_COLUMNS;
    default:
      return DEFAULT_COLUMNS;
  }
}
