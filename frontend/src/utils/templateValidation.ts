import type {
  Field,
  ValidationError,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// FormattedError is what the toast layer feeds to vue-i18n's t():
// `t(key, args)` resolves to the localized string.
export type FormattedError = {
  key: string;
  args: (string | number)[];
};

// formatError maps a backend ValidationError onto an i18n key + args.
// Mirrors `utils/templateValidation.js` from the original Formidable -
// the backend is authoritative; this is presentation only.
export function formatError(error: ValidationError): FormattedError {
  switch (error.type) {
    case "duplicate-keys":
      return {
        key: "error.template.duplicate_keys",
        args: [(error.keys ?? []).join(", ")],
      };

    case "unmatched-loopstart":
      return {
        key: "error.template.unmatched_loopstart",
        args: [error.field?.key || "?"],
      };

    case "unmatched-loopstop":
      return {
        key: "error.template.unmatched_loopstop",
        args: [error.field?.key || "?"],
      };

    case "nested-loop-not-allowed":
      return {
        key: "error.template.nested_loop_not_allowed",
        args: [error.field?.key || "?"],
      };

    case "excessive-loop-nesting": {
      const path = (error.detail?.path as string) || error.key || "unknown";
      const max = (error.detail?.maxDepth as number) ?? 2;
      return {
        key: "error.template.excessive_loop_nesting",
        args: [path, max],
      };
    }

    case "loop-key-mismatch": {
      const expected = (error.detail?.expectedKey as string) || "?";
      return {
        key: "error.template.loop_key_mismatch",
        args: [error.field?.key || "?", expected],
      };
    }

    case "multiple-primary-keys":
      return {
        key: "error.template.multiple_primary_keys",
        args: [(error.keys ?? []).join(", ")],
      };

    case "missing-guid-for-collection":
      return { key: "error.template.missing_guid_for_collection", args: [] };

    case "multiple-tags-fields":
      return {
        key: "error.template.multiple_tags_fields",
        args: [(error.keys ?? []).join(", ")],
      };

    case "multiple-guid-fields":
      return {
        key: "error.template.multiple_guid_fields",
        args: [(error.keys ?? []).join(", ")],
      };

    case "sequence-needs-collection":
      return { key: "error.template.sequence_needs_collection", args: [] };

    case "multiple-sequence-fields":
      return {
        key: "error.template.multiple_sequence_fields",
        args: [(error.keys ?? []).join(", ")],
      };

    case "presentation-needs-sequence":
      return { key: "error.template.presentation_needs_sequence", args: [] };

    case "reserved-key":
      return {
        key: "error.template.reserved_key",
        args: [
          String(error.detail?.key ?? error.key ?? "?"),
          String(error.detail?.owner ?? "?"),
        ],
      };

    case "invalid-template":
      return { key: "error.template.invalid", args: [error.message || ""] };

    case "api-collection-required":
      return { key: "error.api.collection_required", args: [] };

    case "api-map-invalid":
      return { key: "error.api.map_invalid", args: [] };

    case "api-map-key-required":
      return { key: "error.api.map_key_required", args: [] };

    case "api-map-duplicate-keys":
      return {
        key: "error.api.map_duplicate_keys",
        args: [(error.detail?.dup as string) || ""],
      };

    case "expression-item-non-root":
      return {
        key: "error.template.expression_item_non_root",
        args: [error.key || "?", String(error.detail?.levelScope ?? "?")],
      };

    case "forbidden-attribute":
      return {
        key: "error.template.forbidden_attribute",
        args: [
          String(error.detail?.attr ?? "?"),
          String(error.detail?.type ?? "?"),
          error.key || "?",
        ],
      };

    case "level-scope-mismatch":
      return {
        key: "error.template.level_scope_mismatch",
        args: [
          error.key || "?",
          String(error.detail?.got ?? "?"),
          String(error.detail?.want ?? "?"),
        ],
      };

    case "missing-field-key":
      return {
        key: "error.template.missing_field_key",
        args: [error.field?.type || "?"],
      };

    case "formula-field-missing-source":
      return { key: "error.field.formula_missing_source", args: [] };

    case "formula-field-unknown-source":
      return {
        key: "error.field.formula_unknown_source",
        args: [String(error.detail?.formula_key ?? "?")],
      };

    case "formula-field-missing-target":
      return { key: "error.field.formula_missing_target", args: [] };

    case "formula-field-unknown-target":
      return {
        key: "error.field.formula_unknown_target",
        args: [String(error.detail?.target_key ?? "?")],
      };

    case "formula-field-target-not-root":
      return {
        key: "error.field.formula_target_not_root",
        args: [String(error.detail?.target_key ?? "?")],
      };

    case "formula-field-incompatible-target":
      return {
        key: "error.field.formula_incompatible_target",
        args: [
          String(error.detail?.formula_type ?? "?"),
          String(error.detail?.target_key ?? "?"),
          String(error.detail?.target_type ?? "?"),
        ],
      };

    case "formula-field-bad-trigger":
      return {
        key: "error.field.formula_bad_trigger",
        args: [String(error.detail?.trigger ?? "?")],
      };

    case "missing-field-type":
      return {
        key: "error.template.missing_field_type",
        args: [error.key || "?"],
      };

    case "unknown-field-type":
      return {
        key: "error.template.unknown_field_type",
        args: [String(error.detail?.type ?? "?"), error.key || "?"],
      };

    default:
      return {
        key: "error.template.unknown",
        // Strip the embedded Field - its description / code body can run
        // to thousands of chars and turn the toast into a wall of text.
        args: [error.message || error.type || "unknown"],
      };
  }
}

// FieldDraft extends Field with the editor-only `_originalKey` marker
// the modal sets so "edit existing" can ignore self-collisions.
export type FieldDraft = Field & { _originalKey?: string };

export type FieldValidationResult =
  | { valid: true }
  | { valid: false; reason: string; key?: string; type?: string };

// validateField runs the editor-side checks on a single field while the
// user is composing it in FieldEditModal. Backend re-validates on save -
// these rules just give the immediate "won't save" feedback.
export function validateField(
  field: FieldDraft,
  allFields: Field[] = [],
): FieldValidationResult {
  const rawKey = (field.key || "").trim();
  const currentType = field.type || "text";
  const isEditingExisting = !!field._originalKey;
  const originalKey = field._originalKey || "";

  if (rawKey.length === 0) {
    return { valid: false, reason: "missing-key" };
  }

  if (currentType === "guid" && rawKey !== "id") {
    return { valid: false, reason: "guid-key-must-be-id", key: rawKey };
  }

  if (currentType === "tags") {
    const exists = allFields.some(
      (f) =>
        f.type === "tags" && (!isEditingExisting || f.key !== originalKey),
    );
    if (exists) return { valid: false, reason: "only-one-tags-field" };
  }

  if (currentType === "guid") {
    const exists = allFields.some(
      (f) =>
        f.type === "guid" && (!isEditingExisting || f.key !== originalKey),
    );
    if (exists) return { valid: false, reason: "only-one-guid-field" };
  }

  const isDuplicate = allFields.some(
    (f) => f.key === rawKey && (!isEditingExisting || f.key !== originalKey),
  );
  if (isDuplicate) {
    return { valid: false, reason: "duplicate-key", key: rawKey };
  }

  if (currentType === "loopstart" || currentType === "loopstop") {
    const partnerType =
      currentType === "loopstart" ? "loopstop" : "loopstart";

    const hasAnyLoopForKey = allFields.some(
      (f) =>
        f.key === rawKey &&
        (f.type === "loopstart" || f.type === "loopstop"),
    );

    const hasPartner = allFields.some(
      (f) =>
        f.key === rawKey &&
        f.type === partnerType &&
        (!isEditingExisting ||
          f.key !== originalKey ||
          f.type !== currentType),
    );

    if (hasAnyLoopForKey && !hasPartner) {
      return {
        valid: false,
        reason: "unmatched-loop",
        key: rawKey,
        type: currentType,
      };
    }
  }

  if (currentType === "api") {
    const collection = String((field as { collection?: string }).collection || "").trim();
    if (!collection) {
      return { valid: false, reason: "api-collection-required" };
    }

    const map = (field as { map?: unknown }).map;
    if (map != null) {
      if (!Array.isArray(map)) {
        return { valid: false, reason: "api-map-invalid" };
      }
      const seen = new Set<string>();
      for (const m of map) {
        const k = String((m as { key?: string })?.key || "").trim();
        if (!k) return { valid: false, reason: "api-map-key-required" };
        const kl = k.toLowerCase();
        if (seen.has(kl)) {
          return { valid: false, reason: "api-map-duplicate-keys", key: k };
        }
        seen.add(kl);
      }
    }
  }

  return { valid: true };
}

// fieldErrorToI18n turns a validateField result into a {key, args} pair
// the toast/banner layer can pass to t(). Field-level rules borrow the
// `error.field.*` namespace where it exists, falling back to the
// template-level namespace for shared rules.
export function fieldErrorToI18n(
  result: Exclude<FieldValidationResult, { valid: true }>,
): FormattedError {
  switch (result.reason) {
    case "missing-key":
      return { key: "error.field.missing_key", args: [] };
    case "guid-key-must-be-id":
      return {
        key: "error.field.guid_key_must_be_id",
        args: [result.key || ""],
      };
    case "only-one-tags-field":
      return { key: "error.field.only_one_tags_field", args: [] };
    case "only-one-guid-field":
      return { key: "error.field.only_one_guid_field", args: [] };
    case "duplicate-key":
      return { key: "error.field.duplicate_key", args: [result.key || ""] };
    case "unmatched-loop":
      return {
        key: "error.field.unmatched_loop",
        args: [result.type || "loop", result.key || ""],
      };
    case "api-collection-required":
      return { key: "error.api.collection_required", args: [] };
    case "api-map-invalid":
      return { key: "error.api.map_invalid", args: [] };
    case "api-map-key-required":
      return { key: "error.api.map_key_required", args: [] };
    case "api-map-duplicate-keys":
      return {
        key: "error.api.map_duplicate_keys",
        args: [result.key || ""],
      };
    default:
      return { key: "error.template.unknown", args: [result.reason] };
  }
}
