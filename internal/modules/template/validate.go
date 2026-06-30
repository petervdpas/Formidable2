package template

import (
	"fmt"
	"slices"
	"strings"
)

const maxLoopDepth = 2

// Validate runs all template-level checks and returns the accumulated errors.
func Validate(t *Template) []ValidationError {
	if t == nil || t.Fields == nil {
		return []ValidationError{{
			Type:    "invalid-template",
			Message: "Missing or invalid fields array",
		}}
	}

	canonical := assignLevelScopes(t.Fields)

	var errs []ValidationError

	if dups := duplicateKeys(t.Fields); len(dups) > 0 {
		errs = append(errs, ValidationError{
			Type:    "duplicate-keys",
			Keys:    dups,
			Message: fmt.Sprintf("Duplicate field keys: %s", strings.Join(dups, ", ")),
		})
	}

	if e := primaryKeyError(t.Fields); e != nil {
		errs = append(errs, *e)
	}

	errs = append(errs, loopPairingErrors(t.Fields)...)
	errs = append(errs, loopNestingErrors(t.Fields, maxLoopDepth)...)

	if e := collectionGuidError(t); e != nil {
		errs = append(errs, *e)
	}
	if e := singleTagsError(t.Fields); e != nil {
		errs = append(errs, *e)
	}
	if e := singleGuidError(t.Fields); e != nil {
		errs = append(errs, *e)
	}
	if e := sequenceCollectionError(t); e != nil {
		errs = append(errs, *e)
	}
	if e := singleSequenceError(t.Fields); e != nil {
		errs = append(errs, *e)
	}
	errs = append(errs, apiFieldErrors(t.Fields)...)
	errs = append(errs, apiGroupOnNonApiErrors(t.Fields)...)
	errs = append(errs, missingKeyErrors(t.Fields)...)
	errs = append(errs, unknownTypeErrors(t.Fields)...)
	errs = append(errs, forbiddenAttributeErrors(t.Fields)...)
	errs = append(errs, levelScopeMismatchErrors(t.Fields, canonical)...)
	errs = append(errs, expressionItemLevelScopeErrors(canonical)...)
	errs = append(errs, facetsErrors(t.Facets)...)
	errs = append(errs, facetFieldErrors(t)...)
	errs = append(errs, formulaFieldErrors(t, canonical)...)
	errs = append(errs, formulasErrors(t)...)
	errs = append(errs, scalingsErrors(t)...)

	return errs
}

// missingKeyErrors flags any field with an empty key: every field needs one as
// its identifier and (for data fields) its storage slot. guid is exempt because
// Normalize auto-keys it to "id".
func missingKeyErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	for i := range fields {
		f := fields[i]
		if f.Type == "guid" {
			continue
		}
		if strings.TrimSpace(f.Key) == "" {
			ff := f
			errs = append(errs, ValidationError{
				Type:    "missing-field-key",
				Field:   &ff,
				Index:   i,
				Message: fmt.Sprintf("Field #%d (type %q) is missing a key", i+1, f.Type),
			})
		}
	}
	return errs
}

// ValidateFieldDraft returns only the validation errors a candidate field would
// introduce into tpl: it validates tpl with the field applied (replacing the
// field keyed originalKey when editing, or appended when new) and keeps the
// errors that concern the candidate. Empty result means the field is safe to
// confirm. The editor gates its Confirm button on this, so the backend stays the
// single source of validation truth (schema + template: duplicate/missing keys,
// bindings, type/level rules).
func ValidateFieldDraft(tpl *Template, candidate Field, originalKey string, isNew bool) []ValidationError {
	if tpl == nil {
		tpl = &Template{}
	}
	var fields []Field
	insertedIndex := -1
	if !isNew {
		for _, f := range tpl.Fields {
			if insertedIndex < 0 && f.Key == originalKey {
				insertedIndex = len(fields)
				fields = append(fields, candidate)
				continue
			}
			fields = append(fields, f)
		}
	} else {
		fields = append(fields, tpl.Fields...)
	}
	if insertedIndex < 0 {
		insertedIndex = len(fields)
		fields = append(fields, candidate)
	}

	cand := *tpl
	cand.Fields = fields
	var out []ValidationError
	for _, e := range Validate(&cand) {
		if e.Field != nil {
			// Field-specific error: identified by the candidate's position.
			if e.Index == insertedIndex {
				out = append(out, e)
			}
			continue
		}
		// Template-level error (duplicate/primary keys): identified by the candidate's key.
		if candidate.Key != "" && (e.Key == candidate.Key || slices.Contains(e.Keys, candidate.Key)) {
			out = append(out, e)
		}
	}
	return out
}

// scalingsErrors flags structural problems with the scaling (weighting)
// catalog: a bad or empty name, a duplicate name, a missing source key, or a
// source that is not a per-form facet/field (a table column has no single
// per-form weight). Scaling names live in the S["name"] namespace, separate
// from F[], so they may match a field or facet key.
func scalingsErrors(t *Template) []ValidationError {
	if t == nil || len(t.Scalings) == 0 {
		return nil
	}
	var errs []ValidationError
	seen := map[string]bool{}
	for i, s := range t.Scalings {
		if !facetKeyRe.MatchString(s.Name) {
			errs = append(errs, ValidationError{
				Type: "invalid-scaling-name", Index: i, Key: s.Name,
				Message: fmt.Sprintf("Scaling name %q must match ^[a-z][a-z0-9_-]*$", s.Name),
			})
		} else if seen[s.Name] {
			errs = append(errs, ValidationError{
				Type: "duplicate-scaling-name", Index: i, Key: s.Name,
				Message: fmt.Sprintf("Duplicate scaling name %q", s.Name),
			})
		} else {
			seen[s.Name] = true
		}
		if strings.TrimSpace(s.Source.Key) == "" {
			errs = append(errs, ValidationError{
				Type: "scaling-missing-source", Index: i, Key: s.Name,
				Message: fmt.Sprintf("Scaling %q has no source", s.Name),
			})
		}
		if s.Source.Kind != "" && s.Source.Kind != "field" && s.Source.Kind != "facet" {
			errs = append(errs, ValidationError{
				Type: "invalid-scaling-source", Index: i, Key: s.Name,
				Message: fmt.Sprintf("Scaling %q source kind %q invalid (want field or facet)", s.Name, s.Source.Kind),
			})
		}
		if s.Source.Column != "" {
			errs = append(errs, ValidationError{
				Type: "invalid-scaling-source", Index: i, Key: s.Name,
				Message: fmt.Sprintf("Scaling %q source must be a per-form facet or scalar field, not a table column", s.Name),
			})
		}
	}
	return errs
}

var validFormulaTypes = map[string]bool{"number": true, "text": true, "date": true, "bool": true}

// formulasErrors flags structural problems with the formula catalog: a bad or
// empty key, an empty expression, a duplicate key, an unknown result type, or a
// key that collides with a field or facet key (formulas share the F["key"]
// namespace in datacore, so a collision would shadow the real field).
func formulasErrors(t *Template) []ValidationError {
	if t == nil || len(t.Formulas) == 0 {
		return nil
	}
	fieldKeys := map[string]bool{}
	for _, f := range t.Fields {
		if f.Key != "" {
			fieldKeys[f.Key] = true
		}
	}
	facetKeys := map[string]bool{}
	for _, f := range t.Facets {
		if f.Key != "" {
			facetKeys[f.Key] = true
		}
	}
	var errs []ValidationError
	seen := map[string]bool{}
	for i, f := range t.Formulas {
		if !facetKeyRe.MatchString(f.Key) {
			errs = append(errs, ValidationError{
				Type: "invalid-formula-key", Index: i, Key: f.Key,
				Message: fmt.Sprintf("Formula key %q must match ^[a-z][a-z0-9_-]*$", f.Key),
			})
		} else if seen[f.Key] {
			errs = append(errs, ValidationError{
				Type: "duplicate-formula-key", Index: i, Key: f.Key,
				Message: fmt.Sprintf("Duplicate formula key %q", f.Key),
			})
		} else if fieldKeys[f.Key] || facetKeys[f.Key] {
			errs = append(errs, ValidationError{
				Type: "formula-key-collision", Index: i, Key: f.Key,
				Message: fmt.Sprintf("Formula key %q collides with an existing field or facet key", f.Key),
			})
		} else {
			seen[f.Key] = true
		}
		if strings.TrimSpace(f.Expression) == "" {
			errs = append(errs, ValidationError{
				Type: "formula-missing-expression", Index: i, Key: f.Key,
				Message: fmt.Sprintf("Formula %q has no expression", f.Key),
			})
		}
		if f.Type != "" && !validFormulaTypes[f.Type] {
			errs = append(errs, ValidationError{
				Type: "invalid-formula-type", Index: i, Key: f.Key,
				Message: fmt.Sprintf("Formula %q has unknown type %q (want number/text/date/bool)", f.Key, f.Type),
			})
		}
	}
	return errs
}

// facetFieldErrors flags virtual facet fields with a missing/unknown FacetKey or a bad Format.
// Empty Format is accepted: Normalize coerces it to "radio", but Validate runs before Normalize on import paths.
func facetFieldErrors(t *Template) []ValidationError {
	if t == nil {
		return nil
	}
	declared := map[string]bool{}
	for _, f := range t.Facets {
		if f.Key != "" {
			declared[f.Key] = true
		}
	}
	var errs []ValidationError
	for i := range t.Fields {
		f := t.Fields[i]
		if f.Type != "facet" {
			continue
		}
		ff := f
		if f.FacetKey == "" {
			errs = append(errs, ValidationError{
				Type:    "facet-field-missing-key",
				Field:   &ff,
				Index:   i,
				Key:     f.Key,
				Message: "Facet field is missing facet_key",
			})
		} else if !declared[f.FacetKey] {
			errs = append(errs, ValidationError{
				Type:    "facet-field-unknown-key",
				Field:   &ff,
				Index:   i,
				Key:     f.Key,
				Detail:  map[string]any{"facet_key": f.FacetKey},
				Message: "Facet field references unknown facet: " + f.FacetKey,
			})
		}
		if f.Format != "" && !facetFormats[f.Format] {
			errs = append(errs, ValidationError{
				Type:    "facet-field-bad-format",
				Field:   &ff,
				Index:   i,
				Key:     f.Key,
				Detail:  map[string]any{"format": f.Format},
				Message: "Facet field format must be radio or dropdown; got: " + f.Format,
			})
		}
		if declared[f.FacetKey] {
			def, _ := f.Default.(string)
			switch {
			case strings.TrimSpace(def) == "":
				// A bound facet field must declare a default, else forms can never auto-fill it.
				errs = append(errs, ValidationError{
					Type:    "facet-field-missing-default",
					Field:   &ff,
					Index:   i,
					Key:     f.Key,
					Detail:  map[string]any{"facet_key": f.FacetKey},
					Message: "Facet field must declare a default option of facet " + f.FacetKey,
				})
			case !facetHasOptionLabel(t.Facets, f.FacetKey, def):
				errs = append(errs, ValidationError{
					Type:    "facet-field-bad-default",
					Field:   &ff,
					Index:   i,
					Key:     f.Key,
					Detail:  map[string]any{"default": def, "facet_key": f.FacetKey},
					Message: "Facet field default " + def + " is not an option of facet " + f.FacetKey,
				})
			}
		}
	}
	return errs
}

// formulaTargetTypes maps a formula result type to the field types its output
// may be written into. The single source for both validation and the editor's
// target picker (exposed via FormulaTargetTypes). A blank formula type is
// treated as "number" (Normalize's default), so callers normalise before lookup.
var formulaTargetTypes = map[string][]string{
	"number": {"number", "range"},
	"text":   {"text", "textarea"},
	"date":   {"date"},
	"bool":   {"boolean"},
}

// FormulaTargetTypes returns a copy of the formula-type -> acceptable
// field-type map, so the frontend can scope its target picker without
// duplicating the rule.
func FormulaTargetTypes() map[string][]string {
	out := make(map[string][]string, len(formulaTargetTypes))
	for k, v := range formulaTargetTypes {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func formulaTargetAccepts(formulaType, fieldType string) bool {
	if formulaType == "" {
		formulaType = "number"
	}
	return slices.Contains(formulaTargetTypes[formulaType], fieldType)
}

// formulaFieldErrors flags virtual formula fields with a missing/unknown source
// formula, a missing/unknown target (the target must be a real data field, since
// the formula's output is written into its slot), a target whose type can't hold
// the formula's result, a target nested inside a loop (the engine evaluates
// whole-form context only, so targets must be root/level-0), or a bad trigger.
// Empty trigger is accepted: Normalize coerces it to "save", but Validate runs
// before Normalize on import paths. canonical carries the assigned LevelScopes.
func formulaFieldErrors(t *Template, canonical []Field) []ValidationError {
	if t == nil {
		return nil
	}
	formulaType := map[string]string{}
	for _, f := range t.Formulas {
		if f.Key != "" {
			formulaType[f.Key] = f.Type
		}
	}
	dataFieldType := map[string]string{}
	dataFieldLevel := map[string]int{}
	for _, f := range canonical {
		if f.Key == "" || IsVirtualFieldType(f.Type) {
			continue
		}
		if def, ok := fieldDescriptors[f.Type]; ok && def.MetaOnly {
			continue
		}
		dataFieldType[f.Key] = f.Type
		dataFieldLevel[f.Key] = f.LevelScope
	}
	var errs []ValidationError
	for i := range t.Fields {
		f := t.Fields[i]
		if f.Type != "formula" {
			continue
		}
		ff := f
		fType, sourceKnown := formulaType[f.FormulaKey]
		if f.FormulaKey == "" {
			errs = append(errs, ValidationError{
				Type: "formula-field-missing-source", Field: &ff, Index: i, Key: f.Key,
				Message: "Formula field is missing formula_key",
			})
		} else if !sourceKnown {
			errs = append(errs, ValidationError{
				Type: "formula-field-unknown-source", Field: &ff, Index: i, Key: f.Key,
				Detail:  map[string]any{"formula_key": f.FormulaKey},
				Message: "Formula field references unknown formula: " + f.FormulaKey,
			})
		}
		tType, targetKnown := dataFieldType[f.TargetKey]
		if f.TargetKey == "" {
			errs = append(errs, ValidationError{
				Type: "formula-field-missing-target", Field: &ff, Index: i, Key: f.Key,
				Message: "Formula field is missing target_key",
			})
		} else if !targetKnown {
			errs = append(errs, ValidationError{
				Type: "formula-field-unknown-target", Field: &ff, Index: i, Key: f.Key,
				Detail:  map[string]any{"target_key": f.TargetKey},
				Message: "Formula field target is not a data field: " + f.TargetKey,
			})
		} else if dataFieldLevel[f.TargetKey] > 0 {
			// Formulas evaluate whole-form (top-level) context only; a looped
			// target is an array slot a scalar result would corrupt.
			errs = append(errs, ValidationError{
				Type: "formula-field-target-not-root", Field: &ff, Index: i, Key: f.Key,
				Detail:  map[string]any{"target_key": f.TargetKey, "level_scope": dataFieldLevel[f.TargetKey]},
				Message: "Formula field target must be a top-level field, not inside a loop: " + f.TargetKey,
			})
		} else if sourceKnown && !formulaTargetAccepts(fType, tType) {
			errs = append(errs, ValidationError{
				Type: "formula-field-incompatible-target", Field: &ff, Index: i, Key: f.Key,
				Detail:  map[string]any{"formula_type": fType, "target_key": f.TargetKey, "target_type": tType},
				Message: "Formula result type does not fit target field type: " + f.TargetKey,
			})
		}
		if f.Trigger != "" && !formulaTriggers[f.Trigger] {
			errs = append(errs, ValidationError{
				Type: "formula-field-bad-trigger", Field: &ff, Index: i, Key: f.Key,
				Detail:  map[string]any{"trigger": f.Trigger},
				Message: "Formula field trigger must be load, save, or live; got: " + f.Trigger,
			})
		}
	}
	return errs
}

// facetHasOptionLabel reports whether facet key declares an option whose label is def.
func facetHasOptionLabel(facets []Facet, key, def string) bool {
	for _, fc := range facets {
		if fc.Key != key {
			continue
		}
		for _, o := range fc.Options {
			if o.Label == def {
				return true
			}
		}
		return false
	}
	return false
}

// apiGroupOnNonApiErrors flags collection/map populated on a non-api field (dead data that confuses consumers).
func apiGroupOnNonApiErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	for i := range fields {
		f := fields[i]
		if f.Type == "api" {
			continue
		}
		if f.Collection == "" && len(f.Map) == 0 && f.Filter == nil {
			continue
		}
		ff := f
		errs = append(errs, ValidationError{
			Type:    "forbidden-attribute",
			Field:   &ff,
			Index:   i,
			Key:     f.Key,
			Detail:  map[string]any{"attr": "api", "type": f.Type},
			Message: "Attribute api is not allowed on field type " + f.Type,
		})
	}
	return errs
}

// unknownTypeErrors flags fields whose type is missing or not in the registry.
func unknownTypeErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	for i := range fields {
		f := fields[i]
		if f.Type == "" {
			ff := f
			errs = append(errs, ValidationError{
				Type:    "missing-field-type",
				Field:   &ff,
				Index:   i,
				Key:     f.Key,
				Message: "Field is missing a type",
			})
			continue
		}
		if !IsKnownFieldType(f.Type) {
			ff := f
			errs = append(errs, ValidationError{
				Type:    "unknown-field-type",
				Field:   &ff,
				Index:   i,
				Key:     f.Key,
				Detail:  map[string]any{"type": f.Type},
				Message: "Unknown field type: " + f.Type,
			})
		}
	}
	return errs
}

// forbiddenAttributeErrors flags fields carrying properties the registry forbids for their type.
func forbiddenAttributeErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	for i := range fields {
		f := fields[i]
		def, ok := fieldDescriptors[f.Type]
		if !ok {
			// Unknown type already reported by unknownTypeErrors; skip to avoid a flood.
			continue
		}
		for _, attr := range allEnforcedAttrs {
			if def.Abilities.abilityFor(attr) {
				continue
			}
			if !propertyIsSet(f, attr) {
				continue
			}
			ff := f
			errs = append(errs, ValidationError{
				Type:    "forbidden-attribute",
				Field:   &ff,
				Index:   i,
				Key:     f.Key,
				Detail:  map[string]any{"attr": attr, "type": f.Type},
				Message: "Attribute " + attr + " is not allowed on field type " + f.Type,
			})
		}
	}
	return errs
}

// duplicateKeys returns keys appearing more than once, ignoring loopstart/loopstop pairs that legally share a key.
func duplicateKeys(fields []Field) []string {
	seen := map[string]string{}
	var dups []string
	for _, f := range fields {
		if f.Key == "" {
			continue
		}
		if existing, ok := seen[f.Key]; ok {
			isLoopPair :=
				(f.Type == "loopstart" && existing == "loopstop") ||
					(f.Type == "loopstop" && existing == "loopstart")
			if !isLoopPair {
				dups = append(dups, f.Key)
			}
		} else {
			seen[f.Key] = f.Type
		}
	}
	return dups
}

func primaryKeyError(fields []Field) *ValidationError {
	var pkKeys []string
	for _, f := range fields {
		if f.PrimaryKey {
			pkKeys = append(pkKeys, f.Key)
		}
	}
	if len(pkKeys) > 1 {
		return &ValidationError{
			Type:    "multiple-primary-keys",
			Keys:    pkKeys,
			Message: fmt.Sprintf("Multiple primary keys found: %s", strings.Join(pkKeys, ", ")),
		}
	}
	return nil
}

// loopPairingErrors reports unmatched loopstart/loopstop and loop key mismatches.
func loopPairingErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	type frame struct {
		field Field
		index int
	}
	var stack []frame

	for i := range fields {
		f := fields[i]
		switch f.Type {
		case "loopstart":
			stack = append(stack, frame{field: f, index: i})
		case "loopstop":
			if len(stack) == 0 {
				ff := f
				errs = append(errs, ValidationError{
					Type:    "unmatched-loopstop",
					Field:   &ff,
					Index:   i,
					Message: "Unmatched loopstop without preceding loopstart",
				})
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if f.Key != top.field.key() {
				ff := f
				errs = append(errs, ValidationError{
					Type:  "loop-key-mismatch",
					Field: &ff,
					Index: i,
					Detail: map[string]any{
						"expectedKey": top.field.Key,
						"actualKey":   f.Key,
					},
					Message: fmt.Sprintf(
						"Loopstop key %q does not match loopstart key %q",
						f.Key, top.field.Key,
					),
				})
			}
		}
	}

	for _, top := range stack {
		ff := top.field
		errs = append(errs, ValidationError{
			Type:    "unmatched-loopstart",
			Field:   &ff,
			Index:   top.index,
			Message: "Unmatched loopstart without corresponding loopstop",
		})
	}

	return errs
}

func loopNestingErrors(fields []Field, maxDepth int) []ValidationError {
	var errs []ValidationError
	var stack []string
	for _, f := range fields {
		switch f.Type {
		case "loopstart":
			stack = append(stack, f.Key)
			if len(stack) > maxDepth {
				errs = append(errs, ValidationError{
					Type: "excessive-loop-nesting",
					Key:  f.Key,
					Detail: map[string]any{
						"depth":    len(stack),
						"maxDepth": maxDepth,
						"path":     strings.Join(stack, " → "),
					},
					Message: fmt.Sprintf(
						"Loop nesting exceeds %d at key %q (depth %d)",
						maxDepth, f.Key, len(stack),
					),
				})
			}
		case "loopstop":
			if len(stack) > 0 && stack[len(stack)-1] == f.Key {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return errs
}

func collectionGuidError(t *Template) *ValidationError {
	if !t.EnableCollection {
		return nil
	}
	for _, f := range t.Fields {
		if f.Type == "guid" {
			return nil
		}
	}
	return &ValidationError{
		Type:    "missing-guid-for-collection",
		Message: "`Enable Collection` is active but no GUID field found. Add a GUID field or disable this option.",
	}
}

func singleTagsError(fields []Field) *ValidationError {
	var keys []string
	for _, f := range fields {
		if f.Type == "tags" {
			k := f.Key
			if k == "" {
				k = "(no key)"
			}
			keys = append(keys, k)
		}
	}
	if len(keys) > 1 {
		return &ValidationError{
			Type:    "multiple-tags-fields",
			Keys:    keys,
			Message: fmt.Sprintf("Only one 'tags' field is allowed per template (found: %s)", strings.Join(keys, ", ")),
		}
	}
	return nil
}

// singleGuidError flags more than one guid field; two would make the wiki/API resolver's identity ambiguous.
func singleGuidError(fields []Field) *ValidationError {
	var keys []string
	for _, f := range fields {
		if f.Type == "guid" {
			k := f.Key
			if k == "" {
				k = "(no key)"
			}
			keys = append(keys, k)
		}
	}
	if len(keys) > 1 {
		return &ValidationError{
			Type:    "multiple-guid-fields",
			Keys:    keys,
			Message: fmt.Sprintf("Only one 'guid' field is allowed per template (found: %s)", strings.Join(keys, ", ")),
		}
	}
	return nil
}

// sequenceCollectionError flags a sequence field on a non-collection template.
// A sequence orders a set of records, so it only means something once the
// template is a collection (the guid -> collection -> sequence ladder). The
// gate is asymmetric: turning collection off while a sequence field exists
// surfaces here so the author re-enables collection or drops the field.
func sequenceCollectionError(t *Template) *ValidationError {
	hasSequence := false
	for _, f := range t.Fields {
		if f.Type == "sequence" {
			hasSequence = true
			break
		}
	}
	if !hasSequence || t.EnableCollection {
		return nil
	}
	return &ValidationError{
		Type:    "sequence-needs-collection",
		Message: "A `sequence` field needs `Enable Collection`. Enable collection mode or remove the sequence field.",
	}
}

// singleSequenceError flags more than one sequence field; a collection has one
// authored order, so two sequences would make "the order" ambiguous.
func singleSequenceError(fields []Field) *ValidationError {
	var keys []string
	for _, f := range fields {
		if f.Type == "sequence" {
			k := f.Key
			if k == "" {
				k = "(no key)"
			}
			keys = append(keys, k)
		}
	}
	if len(keys) > 1 {
		return &ValidationError{
			Type:    "multiple-sequence-fields",
			Keys:    keys,
			Message: fmt.Sprintf("Only one 'sequence' field is allowed per template (found: %s)", strings.Join(keys, ", ")),
		}
	}
	return nil
}

func apiFieldErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	for _, f := range fields {
		if f.Type != "api" {
			continue
		}
		ff := f
		key := f.Key
		if key == "" {
			key = "(no key)"
		}
		if strings.TrimSpace(f.Collection) == "" {
			errs = append(errs, ValidationError{
				Type:    "api-collection-required",
				Field:   &ff,
				Key:     key,
				Message: "API field requires a non-empty collection name.",
			})
			continue
		}
		if f.Map != nil {
			seen := map[string]bool{}
			for _, m := range f.Map {
				k := strings.TrimSpace(m.Key)
				if k == "" {
					errs = append(errs, ValidationError{
						Type:    "api-map-key-required",
						Field:   &ff,
						Key:     key,
						Message: "Each API map entry must have a non-empty 'key'.",
					})
					break
				}
				kl := strings.ToLower(k)
				if seen[kl] {
					errs = append(errs, ValidationError{
						Type:    "api-map-duplicate-keys",
						Field:   &ff,
						Key:     key,
						Detail:  map[string]any{"dup": k},
						Message: fmt.Sprintf("Duplicate API map key: %s", k),
					})
					break
				}
				seen[kl] = true
			}
		}
		if f.Filter != nil {
			if strings.TrimSpace(f.Filter.FieldKey) == "" {
				errs = append(errs, ValidationError{
					Type:    "api-filter-field-required",
					Field:   &ff,
					Key:     key,
					Message: "API field filter requires a target field.",
				})
			} else if !apiFilterOps[f.Filter.Op] {
				errs = append(errs, ValidationError{
					Type:    "api-filter-op-invalid",
					Field:   &ff,
					Key:     key,
					Detail:  map[string]any{"op": f.Filter.Op},
					Message: fmt.Sprintf("Invalid API field filter operator: %q", f.Filter.Op),
				})
			}
		}
	}
	return errs
}

// apiFilterOps is the allowed operator set for an api field filter (mirrors the
// query module's FilterOps).
var apiFilterOps = map[string]bool{
	"eq": true, "ne": true, "gt": true, "ge": true, "lt": true, "le": true,
}

func (f Field) key() string { return f.Key }
