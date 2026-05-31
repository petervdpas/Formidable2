package builder

import (
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// IsDisplayableFieldType reports whether a type may appear in an Outcome's display parts: a known,
// non-virtual type. Virtual fields render through a dedicated widget, so concatenating them into a label is redundant.
func IsDisplayableFieldType(fieldType string) bool {
	if !template.IsKnownFieldType(fieldType) {
		return false
	}
	return !template.IsVirtualFieldType(fieldType)
}

// OptionsForField returns the value/label pairs for the value-picker. A facet field resolves through its
// bound facet's option labels (stored verbatim in meta.facets); other types read f.Options; non-enum types yield empty.
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
