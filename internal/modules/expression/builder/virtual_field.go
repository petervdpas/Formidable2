package builder

import (
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// IsDisplayableFieldType reports whether a field type may appear in
// an Outcome's display parts (FieldValue / FieldLabel pickers). The
// rule is intentionally narrow: a known field type is displayable
// unless its registry entry marks it virtual. Virtual fields project
// their value through a dedicated widget (a facet chip, a future
// derived-field readout) and concatenating that value into a label
// string is redundant. They remain available as predicate criteria
// via KindForField.
//
// Returning false for unknown types is the conservative path - the
// frontend display pickers already filter by expression_item, so an
// unknown type slipping through here would have no rendering
// contract to honor anyway.
func IsDisplayableFieldType(fieldType string) bool {
	if !template.IsKnownFieldType(fieldType) {
		return false
	}
	return !template.IsVirtualFieldType(fieldType)
}

// OptionsForField returns the value/label pairs the predicate
// value-picker should offer for the given field. Backend owns the
// projection so virtual types (facet → bound facet's option labels)
// resolve identically wherever the frontend asks.
//
// Behavior per type:
//   - facet: look up f.FacetKey in facets; each option becomes
//     {Value: label, Label: label} since meta.facets stores the label
//     verbatim. Returns empty when the binding is missing.
//   - dropdown / radio / multioption / boolean / range / list / table:
//     read f.Options entries. Map entries respect {value, label} with
//     label falling back to value when empty. Plain strings become
//     {Value: s, Label: s}.
//   - everything else: empty slice (no enumeration to picker against).
func OptionsForField(f template.Field, facets []template.Facet) []FieldOption {
	if f.Type == "facet" {
		for _, fc := range facets {
			if fc.Key != f.FacetKey {
				continue
			}
			out := make([]FieldOption, 0, len(fc.Options))
			for _, o := range fc.Options {
				out = append(out, FieldOption{Value: o.Label, Label: o.Label})
			}
			return out
		}
		return []FieldOption{}
	}
	out := make([]FieldOption, 0, len(f.Options))
	for _, raw := range f.Options {
		switch v := raw.(type) {
		case map[string]any:
			value, _ := v["value"].(string)
			label, _ := v["label"].(string)
			if label == "" {
				label = value
			}
			out = append(out, FieldOption{Value: value, Label: label})
		case string:
			out = append(out, FieldOption{Value: v, Label: v})
		}
	}
	return out
}
