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
