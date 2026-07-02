import type {
  ColumnDef,
  FixedRowConfig,
  OptionRow,
  SubRowConfig,
  SubRowVariant,
} from "../components/fields/OptionsEditor.vue";
import {
  Service as TemplateSvc,
  type FieldDescriptor,
  type TableColumnTypeDescriptor,
  type SubRow as BackendSubRow,
  type FixedOptionsShape,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Per-field-type column presets for the OptionsEditor. Mirrors
// `utils/optionsEditor.js` from the original Formidable.
//
// Backend ownership: list/table column-type vocabularies live on the
// Go side (TemplateSvc.ListItemTypes / TemplateSvc.TableColumnTypes
// - see internal/modules/template/option_presets.go). This module
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
  "number",
  "range",
  "slide",
  "slideset",
  "list",
  "table",
  // file-path uses options to declare allowed extension filters
  // ("*.json", "*.md") that drive the native picker's filter dropdown.
  "file-path",
]);

// Bootstrap fallbacks. Used until ensureOptionPresetsLoaded resolves
// (or if it fails - degraded but functional). Match the canonical
// builtinTableColumnTypes / builtinListItemTypes in Go.
let _tableColumnTypes: TableColumnTypeDescriptor[] = [];
let _listItemTypes: string[] = ["fixed", "custom"];
let _slideFormats: string[] = ["1280 x 720 (16:9)", "1920 x 1080 (16:9)", "1024 x 768 (4:3)"];
let _fieldDescriptors: Record<string, FieldDescriptor> = {};

let loadPromise: Promise<void> | null = null;

async function load(): Promise<void> {
  try {
    const [tcols, ltypes, ftypes, sfmts] = await Promise.all([
      TemplateSvc.TableColumnTypes(),
      TemplateSvc.ListItemTypes(),
      TemplateSvc.FieldTypes(),
      TemplateSvc.SlideFormats(),
    ]);
    if (tcols && tcols.length > 0) {
      _tableColumnTypes = tcols;
    }
    if (ltypes && ltypes.length > 0) {
      _listItemTypes = ltypes.map((d) => d.name);
    }
    if (sfmts && sfmts.length > 0) {
      _slideFormats = sfmts;
    }
    if (ftypes && ftypes.length > 0) {
      _fieldDescriptors = {};
      for (const d of ftypes) _fieldDescriptors[d.id] = d;
    }
  } catch {
    // Stay on the bootstrap fallbacks - better empty UX than crash.
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
        // Switching back from custom - unlock and clear the placeholder.
        const cleared = row["value"] === "[[custom]]" ? "" : row["value"];
        return { ...row, value: cleared, _valueLocked: false };
      },
    },
    { key: "value", type: "text", placeholder: "value" },
    { key: "label", type: "text", placeholder: "label" },
  ];
}

// Translate the backend's SubRow record into the editor-facing
// SubRowVariant. Pure shape conversion - labels travel as i18n keys
// (resolved in OptionsSubRow via vue-i18n) so no English text leaks
// across the Wails boundary.
function toSubRowVariant(s: BackendSubRow): SubRowVariant {
  const v: SubRowVariant = {};
  if (s.row_key) v.rowKey = s.row_key;
  if (s.label_key) v.labelKey = s.label_key;
  if (s.placeholder_key) v.placeholderKey = s.placeholder_key;
  if (s.scalar) v.scalar = true;
  if (s.default) v.defaultValue = s.default;
  if (s.max_entries && s.max_entries > 0) v.maxEntries = s.max_entries;
  if (s.entries && s.entries.length > 0) {
    v.entries = s.entries.map((e) => ({
      labelKey: e.label_key,
      value: e.value,
      placeholderKey: e.placeholder_key,
    }));
  }
  return v;
}

// Build a SubRowConfig from the per-column-type SubRows the backend
// advertised. Each variant carries its own row_key (choices for
// bool/dropdown, step for number), so distinct column types can store
// into distinct row keys. The config-level rowKey is a fallback only.
// Empty (no column type has a SubRow) returns undefined so the
// dropdown ColumnDef stays sub-row-less.
function tableTypeSubRowConfig(
  cols: TableColumnTypeDescriptor[],
): SubRowConfig | undefined {
  const perValue: Record<string, SubRowVariant> = {};
  let rowKey = "";
  for (const c of cols) {
    if (!c.sub_row) continue;
    if (!rowKey) rowKey = c.sub_row.row_key;
    perValue[c.name] = toSubRowVariant(c.sub_row);
  }
  if (!rowKey || Object.keys(perValue).length === 0) return undefined;
  return { rowKey, perValue };
}

// Table columns: key + type + label. The `type` dropdown's sub-row
// config (and the column-type vocabulary itself) comes from the Go
// builtinTableColumnTypes descriptors via TemplateSvc.
function tableColumns(): ColumnDef[] {
  const names = _tableColumnTypes.map((d) => d.name);
  return [
    { key: "value", type: "text", placeholder: "key" },
    {
      key: "type",
      type: "dropdown",
      options: names,
      defaultValue: names[0] ?? "string",
      placeholder: "type",
      subRow: tableTypeSubRowConfig(_tableColumnTypes),
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

// The slide field's single canvas_format row: value is locked, and its label
// cell is a dropdown of the backend-owned format presets (aspect ratio + size).
function slideColumns(): ColumnDef[] {
  return [
    { key: "value", type: "text", placeholder: "value" },
    {
      key: "label",
      type: "dropdown",
      options: [..._slideFormats],
      defaultValue: _slideFormats[0] ?? "1280 x 720 (16:9)",
      placeholder: "format",
    },
  ];
}

export function columnsFor(typeId: string): ColumnDef[] | null {
  if (!SUPPORTED_OPTION_TYPES.has(typeId)) return null;
  switch (typeId) {
    case "list":
      return listColumns();
    case "table":
      return tableColumns();
    case "file-path":
      return FILE_PATH_COLUMNS;
    case "slide":
      return slideColumns();
    default:
      return DEFAULT_COLUMNS;
  }
}

// fixedRowsFor returns a structural row template for field types
// whose options have a fixed arity. Null otherwise - the editor
// stays in free-form add/remove mode. Source of truth is the Go
// FieldDescriptor.OptionsShape (see internal/modules/template/
// field_abilities.go); this function is a pure shape adapter.
export function fixedRowsFor(typeId: string): FixedRowConfig[] | null {
  const def = _fieldDescriptors[typeId];
  const shape = def?.options_shape as FixedOptionsShape | null | undefined;
  if (!shape || !shape.rows || shape.rows.length === 0) return null;
  return shape.rows.map((r) => ({
    labelKey: r.label_key,
    defaults: (r.defaults ?? {}) as OptionRow,
  }));
}

// Column keys the editor renders read-only for a fixed-shape type
// (e.g. the structural "value" of boolean's true/false or range's
// min/max/step). Empty for free-form / unlocked types.
export function lockedColumnsFor(typeId: string): string[] {
  const def = _fieldDescriptors[typeId];
  const shape = def?.options_shape as FixedOptionsShape | null | undefined;
  return shape?.locked_columns ? [...shape.locked_columns] : [];
}
