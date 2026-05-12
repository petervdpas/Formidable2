package csv

import "strings"

// FieldSpec is the Wails-friendly subset of template.Field needed by the
// CSV mapping UI. The dialog passes this from the template it already
// has cached on the frontend, so the backend doesn't reload templates.
type FieldSpec struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Label   string `json:"label"`
	Options []any  `json:"options"`
}

// SuggestedMapping is one row of auto-suggested CSV → field assignments.
// Empty FieldKey means "no auto-match found"; the user is expected to
// pick a target manually (or leave it unmapped).
type SuggestedMapping struct {
	Header   string `json:"header"`
	FieldKey string `json:"fieldKey"`
}

// SuggestMappings proposes a target field for each CSV header by
// normalising both sides (lowercase, drop space/underscore/dash) and
// matching first on field.Key, then on field.Label. Excluded field
// types are never suggested.
func SuggestMappings(headers []string, fields []FieldSpec) []SuggestedMapping {
	mappable := MappableFields(fields)
	out := make([]SuggestedMapping, 0, len(headers))
	for _, h := range headers {
		out = append(out, SuggestedMapping{Header: h, FieldKey: pickField(h, mappable)})
	}
	return out
}

// MappableFields filters out field types that cannot participate in
// CSV mapping (image, code, api, loop boundaries).
func MappableFields(fields []FieldSpec) []FieldSpec {
	excluded := make(map[string]bool, len(excludedTypes))
	for _, t := range excludedTypes {
		excluded[t] = true
	}
	out := make([]FieldSpec, 0, len(fields))
	for _, f := range fields {
		if !excluded[f.Type] {
			out = append(out, f)
		}
	}
	return out
}

func pickField(header string, fields []FieldSpec) string {
	norm := normaliseHeader(header)
	if norm == "" {
		return ""
	}
	for _, f := range fields {
		if normaliseHeader(f.Key) == norm {
			return f.Key
		}
	}
	for _, f := range fields {
		if normaliseHeader(f.Label) == norm {
			return f.Key
		}
	}
	return ""
}

// normaliseHeader collapses casing, whitespace, underscores, and dashes
// so that "Unit Number", "unit_number", "unit-number" and "unitnumber"
// all compare equal.
func normaliseHeader(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '\t', '\n', '_', '-':
			// strip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
