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
  virtual?: boolean;
  keyReadonly?: boolean;
  /** Backend-owned (FieldDescriptor.RequiresCollection): the type only
   *  means something on a collection, so the Type dropdown hides it until
   *  Enable Collection is on. Currently just `sequence`. */
  requiresCollection?: boolean;
  /** Backend-owned (FieldDescriptor.RequiresSlide): the type needs a slide
   *  field on the template, so the Type dropdown hides it until one exists.
   *  Currently just `slideset` (decks group slides). */
  requiresSlide?: boolean;
}

const registry: Ref<FieldTypeDef[]> = ref([]);
let loadPromise: Promise<void> | null = null;

// Label key + new-field default value are backend-owned (FieldDescriptor):
// no hand-maintained frontend copy. The default arrives as a static value, so
// we clone it per call to keep the factory contract (fresh array/object each
// time). Meta types have no default (null) -> no factory.
async function load(): Promise<void> {
  const defs = (await TemplateSvc.FieldTypes()) ?? [];
  registry.value = defs.map((d) => ({
    id: d.id,
    labelKey: d.label_key || d.id,
    defaultValue:
      d.default_value == null
        ? undefined
        : () => structuredClone(d.default_value),
    metaOnly: d.meta_only,
    virtual: d.virtual,
    keyReadonly: d.key_readonly,
    // Read defensively: the generated FieldDescriptor type gains this field on
    // the next bindings regen, but Object.assign in createFrom already carries
    // the value through at runtime today.
    requiresCollection:
      (d as { requires_collection?: boolean }).requires_collection === true,
    requiresSlide:
      (d as { requires_slide?: boolean }).requires_slide === true,
    abilities: d.abilities,
  }));
}

// isDataField reports whether a type carries its own Form.Data slot, i.e. it can
// be a formula field's write target. Virtual (facet/formula) and meta-only
// (loop markers) types have no slot.
export function isDataField(typeId: string): boolean {
  const def = getFieldTypeDef(typeId);
  if (!def) return false;
  return !def.virtual && !def.metaOnly;
}

export function ensureFieldTypesLoaded(): Promise<void> {
  if (!loadPromise) loadPromise = load();
  return loadPromise;
}

// formula-result-type -> acceptable target field types (backend-owned, see
// template.FormulaTargetTypes). Scopes a formula field's target picker so a
// text formula can't write into a number field, etc.
const formulaTargets: Ref<Record<string, string[]>> = ref({});
let formulaTargetsPromise: Promise<void> | null = null;

export function ensureFormulaTargetTypesLoaded(): Promise<void> {
  if (!formulaTargetsPromise) {
    formulaTargetsPromise = TemplateSvc.FormulaTargetTypes().then((m) => {
      formulaTargets.value = (m as Record<string, string[]>) ?? {};
    });
  }
  return formulaTargetsPromise;
}

// Acceptable target field types for a formula result type (empty until loaded).
export function formulaTargetFieldTypes(formulaType: string): string[] {
  return formulaTargets.value[formulaType || "number"] ?? [];
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

// Whether the Key input is shown but locked for a type (guid: key is forced
// to "id" by the backend). Backend-owned via FieldDescriptor.KeyReadonly.
export function isKeyReadonly(typeId: string): boolean {
  return getFieldTypeDef(typeId)?.keyReadonly === true;
}

/** Type IDs eligible to appear in the Edit modal's Type dropdown.
 *
 *  - For new fields (isNew): include `looper` (synthesizes a
 *    loopstart/loopstop pair on confirm); hide loopstart/loopstop.
 *  - For existing loopstart/loopstop: lock to that same type - the
 *    user can't "convert" half of a pair into something else.
 *  - For other existing fields: hide `looper`, `loopstart`, `loopstop`.
 *  - Collection-only types (sequence) are hidden unless `enableCollection`
 *    is on, except when the field already is that type - switching an
 *    existing sequence field away is fine, but it must not vanish from its
 *    own dropdown (and the backend still flags the invalid combination). */
export function selectableTypes(
  currentType: string,
  isNew = false,
  enableCollection = false,
  hasSlideField = false,
): FieldTypeDef[] {
  return registry.value.filter((t) => {
    if (t.id === "loopstart" || t.id === "loopstop") {
      return t.id === currentType;
    }
    if (t.id === "looper") {
      return isNew;
    }
    if (t.requiresCollection && !enableCollection) {
      return t.id === currentType;
    }
    // A slideset needs a slide field on the template (decks group slides), so
    // hide it until one exists - except when the field already is that type.
    if (t.requiresSlide && !hasSlideField) {
      return t.id === currentType;
    }
    return true;
  });
}
