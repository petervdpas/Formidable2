// Field-type registry - single source of truth lives in Go
// (internal/modules/template/field_abilities.go). This module loads
// the backend matrix on first call and merges it with the few
// frontend-only display concerns: i18n labelKey + per-type "default
// value when creating a new field".
//
// Public surface intentionally matches the previous static
// `FIELD_TYPES`: getFieldTypeDef / isRowHidden / selectableTypes /
// FIELD_TYPES - so existing consumers keep working without changes.
//
// Boot ordering: ensureFieldTypesLoaded (in main.ts) kicks off load()
// before the app mounts, so by the time any component calls
// selectableTypes() the registry is populated. If the call beats the
// load (shouldn't happen in normal flow), the helpers degrade
// gracefully - the dropdown is empty and isRowHidden returns false.

import { ref, type Ref } from "vue";
import {
  Service as TemplateSvc,
  Abilities,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

/** Stable IDs for every row the Edit Field modal can render: exactly
 *  the keys of the backend `Abilities` matrix. Derived from the
 *  generated type so a new ability flows through without editing a
 *  hand-maintained list here (backend owns the set). */
export type FieldEditRowId = keyof Abilities;

export interface FieldTypeDef {
  id: string;
  labelKey: string;
  defaultValue?: () => unknown;
  abilities: Abilities;
  metaOnly?: boolean;
}

const LABEL_KEYS: Record<string, string> = {
  text: "workspace.templates.field_type.text",
  "file-path": "workspace.templates.field_type.file_path",
  "folder-path": "workspace.templates.field_type.folder_path",
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
  api: "workspace.templates.field_type.api",
  guid: "workspace.templates.field_type.guid",
  facet: "workspace.templates.field_type.facet",
  looper: "workspace.templates.field_type.looper",
  loopstart: "workspace.templates.field_type.loopstart",
  loopstop: "workspace.templates.field_type.loopstop",
};

const DEFAULT_FACTORY: Record<string, () => unknown> = {
  text: () => "",
  "file-path": () => "",
  "folder-path": () => "",
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
  api: () => ({ id: "", overrides: {} }),
  guid: () => "",
};

const registry: Ref<FieldTypeDef[]> = ref([]);
let loadPromise: Promise<void> | null = null;

async function load(): Promise<void> {
  const defs = (await TemplateSvc.FieldTypes()) ?? [];
  registry.value = defs.map((d) => ({
    id: d.id,
    labelKey: LABEL_KEYS[d.id] ?? d.id,
    defaultValue: DEFAULT_FACTORY[d.id],
    metaOnly: d.meta_only,
    abilities: d.abilities,
  }));
}

export function ensureFieldTypesLoaded(): Promise<void> {
  if (!loadPromise) loadPromise = load();
  return loadPromise;
}

export const FIELD_TYPES = registry;

export function getFieldTypeDef(id: string): FieldTypeDef | undefined {
  return registry.value.find((d) => d.id === id);
}

export function isRowHidden(typeId: string, rowId: FieldEditRowId): boolean {
  const def = getFieldTypeDef(typeId);
  if (!def) return false;
  return def.abilities[rowId] === false;
}

/** Type IDs eligible to appear in the Edit modal's Type dropdown.
 *
 *  - For new fields (isNew): include `looper` (synthesizes a
 *    loopstart/loopstop pair on confirm); hide loopstart/loopstop.
 *  - For existing loopstart/loopstop: lock to that same type - the
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
