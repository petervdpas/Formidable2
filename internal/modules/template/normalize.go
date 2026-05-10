package template

import "strings"

// textareaFormats mirrors `schemas/field.schema.js`:
//   const textareaFormats = new Set(["markdown", "plain"]);
var textareaFormats = map[string]bool{
	"markdown": true,
	"plain":    true,
}

// Normalize coerces a Template's fields into the shape the rest of the
// pipeline (and downstream renderers) expects, mirroring the original
// JS field-schema normalizer. Idempotent: safe to call repeatedly.
//
// Currently handles:
//   - textarea: Format is forced to "markdown" or "plain" (default
//     "markdown" when missing/unknown), case-insensitively.
//   - non-textarea: Format is stripped — the YAML omitempty tag then
//     keeps it out of the saved file.
//
// nil Template / nil Fields are no-ops (callers might pass partials).
func Normalize(t *Template) {
	if t == nil {
		return
	}
	for i := range t.Fields {
		normalizeField(&t.Fields[i])
	}
	t.Fields = assignLevelScopes(t.Fields)
	for i := range t.Fields {
		if t.Fields[i].LevelScope > 0 && t.Fields[i].ExpressionItem {
			t.Fields[i].ExpressionItem = false
		}
		if t.Fields[i].Type == "guid" {
			t.Fields[i].Key = "id"
		}
		stripDisabledAttributes(&t.Fields[i])
	}
}

func stripDisabledAttributes(f *Field) {
	def, ok := fieldDescriptors[f.Type]
	if !ok {
		return
	}
	for _, attr := range allEnforcedAttrs {
		if !def.Abilities.abilityFor(attr) {
			clearProperty(f, attr)
		}
	}
}

func clearProperty(f *Field, attr string) {
	switch attr {
	case attrSummaryField:
		f.SummaryField = ""
	case attrPrimaryKey:
		f.PrimaryKey = false
	case attrLabel:
		f.Label = ""
	case attrDescription:
		f.Description = ""
	case attrDefault:
		f.Default = nil
	case attrOptions:
		f.Options = nil
	case attrExpressionItem:
		f.ExpressionItem = false
	case attrTwoColumn:
		f.TwoColumn = false
	case attrCollapsible:
		f.Collapsible = nil
	case attrReadonly:
		f.Readonly = false
	case attrFormat:
		f.Format = ""
	}
}

func normalizeField(f *Field) {
	if f.Type == "textarea" {
		canon := strings.ToLower(strings.TrimSpace(f.Format))
		if !textareaFormats[canon] {
			canon = "markdown"
		}
		f.Format = canon
		return
	}
	// All non-textarea types: format has no meaning, drop it.
	f.Format = ""
}
