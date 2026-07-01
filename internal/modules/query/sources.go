package query

import (
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Sources returns the queryable sources for a template: its fields (table
// columns fanned out), and its facets, each with the capabilities the
// query UI needs. This is the single source of truth for "what can I
// query and how does it behave" - the frontend renders it, never derives
// it.
func (m *Manager) Sources(templateName string) ([]SourceInfo, error) {
	tpl, err := m.loader.Template(templateName)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, fmt.Errorf("query: template %q not found", templateName)
	}
	if tpl.Presentation {
		return nil, ErrPresentationExcluded
	}
	return deriveSources(tpl), nil
}

// skipFieldType lists field types that hold no queryable data (binary,
// structural, or cross-template) so they don't clutter the picker.
var skipFieldType = map[string]bool{
	"image":   true,
	"api":     true,
	"button":  true,
	"facet":   true, // facets are emitted from tpl.Facets, not the bound field
	"heading": true,
}

func deriveSources(tpl *template.Template) []SourceInfo {
	out := make([]SourceInfo, 0)
	for _, f := range tpl.Fields {
		if skipFieldType[f.Type] {
			continue
		}
		label := f.Label
		if label == "" {
			label = f.Key
		}

		switch f.Type {
		case "table":
			for i, opt := range f.Options {
				mp, ok := opt.(map[string]any)
				if !ok {
					continue
				}
				val, _ := mp["value"].(string)
				if val == "" {
					continue
				}
				clabel, _ := mp["label"].(string)
				if clabel == "" {
					clabel = val
				}
				ctype, _ := mp["type"].(string)
				col := i
				src := Source{Kind: "field", Key: f.Key, Col: &col}
				out = append(out, SourceInfo{
					ID:           sourceID(src),
					Label:        label + " / " + clabel,
					Source:       src,
					Numeric:      ctype == "number",
					Date:         ctype == "date",
					Fans:         true,
					Aggregatable: ctype == "number",
				})
			}

		case "list", "tags", "multioption":
			src := Source{Kind: "field", Key: f.Key}
			out = append(out, SourceInfo{
				ID: sourceID(src), Label: label, Source: src, Fans: true,
				Choices: optionChoices(f.Options),
			})

		case "dropdown", "radio":
			src := Source{Kind: "field", Key: f.Key}
			out = append(out, SourceInfo{
				ID: sourceID(src), Label: label, Source: src, Choices: optionChoices(f.Options),
			})

		default:
			src := Source{Kind: "field", Key: f.Key}
			out = append(out, SourceInfo{
				ID:           sourceID(src),
				Label:        label,
				Source:       src,
				Numeric:      f.Type == "number" || f.Type == "range",
				Date:         f.Type == "date",
				Aggregatable: f.Type == "number" || f.Type == "range",
			})
		}
	}

	for _, fc := range tpl.Facets {
		src := Source{Kind: "facet", Key: fc.Key}
		var ch []Choice
		for _, o := range fc.Options {
			if o.Label != "" {
				ch = append(ch, Choice{Value: o.Label, Label: o.Label})
			}
		}
		out = append(out, SourceInfo{ID: sourceID(src), Label: fc.Key, Source: src, Choices: ch})
	}

	return out
}

// optionChoices extracts {value,label} pairs from a dropdown/radio/list
// field's options. Each option is a {value,label} map; a blank value is
// skipped.
func optionChoices(options []any) []Choice {
	var out []Choice
	for _, opt := range options {
		mp, ok := opt.(map[string]any)
		if !ok {
			continue
		}
		val, _ := mp["value"].(string)
		if val == "" {
			continue
		}
		label, _ := mp["label"].(string)
		if label == "" {
			label = val
		}
		out = append(out, Choice{Value: val, Label: label})
	}
	return out
}
