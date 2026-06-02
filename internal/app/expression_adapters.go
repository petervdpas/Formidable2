package app

import (
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// expressionTemplateAdapter satisfies expression.TemplateProvider so the engine
// never imports template directly. The expression-flagged field list backs the
// engine's narrowContext defence.
type expressionTemplateAdapter struct {
	tpl *template.Manager
}

// LookupExpression reads sidebar_expression plus the expression-flagged fields
// with their option metadata. On no-config returns ("", nil, nil): no error
// means "loaded fine, configured empty"; the engine maps empty to ErrNoExpression.
func (a expressionTemplateAdapter) LookupExpression(name string) (string, []expression.ExpressionField, error) {
	t, err := a.tpl.LoadTemplate(name)
	if err != nil {
		return "", nil, err
	}
	fields := make([]expression.ExpressionField, 0, len(t.Fields)+len(t.Formulas))
	for _, f := range t.Fields {
		if !f.ExpressionItem {
			continue
		}
		fields = append(fields, expression.ExpressionField{
			Key:     f.Key,
			Options: optionLabelMap(f.Options),
		})
	}
	// Formula keys are whitelisted too, so the values the harvest folded into
	// the expression context survive narrowContext and a sidebar expression can
	// reference F["formula"]. Formulas carry no options.
	for _, fm := range t.Formulas {
		fields = append(fields, expression.ExpressionField{Key: fm.Key})
	}
	// Facet keys are whitelisted so the harvested facet selected value (folded
	// under the facet key) survives narrowContext and F["facet-key"] resolves.
	for _, fc := range t.Facets {
		fields = append(fields, expression.ExpressionField{Key: fc.Key})
	}
	return t.SidebarExpression, fields, nil
}

// optionLabelMap normalises a template.Field's `Options []any` into a
// value-to-label map for the O[] resolver. Mirrors the render/api option-pair
// convention: string entries become {v:s, l:s}; map entries read "value" +
// "label" with label falling back to value. Non-option fields produce nil.
func optionLabelMap(opts []any) map[string]string {
	if len(opts) == 0 {
		return nil
	}
	out := make(map[string]string, len(opts))
	for _, opt := range opts {
		switch x := opt.(type) {
		case string:
			out[x] = x
		case map[string]any:
			val, _ := x["value"].(string)
			lab, _ := x["label"].(string)
			if val == "" {
				continue
			}
			if lab == "" {
				lab = val
			}
			out[val] = lab
		}
	}
	return out
}

// expressionStorageAdapter satisfies expression.StorageProvider by mapping
// FormSummary to expression.Record. Storage already harvests ExpressionItems on
// the list path; the adapter renames the field and drops Meta.
type expressionStorageAdapter struct {
	sto *storage.Manager
}

func (a expressionStorageAdapter) ListForExpression(templateName string) ([]expression.Record, error) {
	summaries, err := a.sto.ExtendedListForms(templateName)
	if err != nil {
		return nil, err
	}
	out := make([]expression.Record, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, expression.Record{
			Filename: s.Filename,
			Title:    s.Title,
			Context:  s.ExpressionItems,
		})
	}
	return out, nil
}

// LookupForExpression is the per-record analogue used by
// Manager.EvaluateListOne. Missing file yields an empty Record (matches
// storage.ExtendedLoadForm's nil-on-missing posture).
func (a expressionStorageAdapter) LookupForExpression(templateName, datafile string) (expression.Record, error) {
	s, err := a.sto.ExtendedLoadForm(templateName, datafile)
	if err != nil {
		return expression.Record{}, err
	}
	if s == nil {
		return expression.Record{}, nil
	}
	return expression.Record{
		Filename: s.Filename,
		Title:    s.Title,
		Context:  s.ExpressionItems,
	}, nil
}
