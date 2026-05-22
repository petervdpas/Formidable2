package app

import (
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// expressionTemplateAdapter satisfies expression.TemplateProvider so
// the engine module never needs to import template directly. Returns
// the sidebar expression and the list of expression-flagged field
// keys - that field list backs the engine's narrowContext defence.
type expressionTemplateAdapter struct {
	tpl *template.Manager
}

// LookupExpression reads the template's sidebar_expression and the
// list of expression-flagged fields with their option metadata.
// The option map (value→label) backs the engine's per-record O[]
// resolution. Empty expression maps to ErrNoExpression at the
// engine layer; the adapter returns ("", nil, nil) on no-config so
// the contract stays explicit (no error means "loaded fine, but
// configured empty").
func (a expressionTemplateAdapter) LookupExpression(name string) (string, []expression.ExpressionField, error) {
	t, err := a.tpl.LoadTemplate(name)
	if err != nil {
		return "", nil, err
	}
	fields := make([]expression.ExpressionField, 0, len(t.Fields))
	for _, f := range t.Fields {
		if !f.ExpressionItem {
			continue
		}
		fields = append(fields, expression.ExpressionField{
			Key:     f.Key,
			Options: optionLabelMap(f.Options),
		})
	}
	return t.SidebarExpression, fields, nil
}

// optionLabelMap normalises a template.Field's `Options []any` into
// a value→label map for the O[] resolver. Mirrors the option-pair
// convention used by render/api: string entries become {v:s, l:s};
// map entries read "value" + "label" with label falling back to
// value. Anything else stringifies. Non-option-bearing fields
// produce an empty (nil-safe) map.
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

// expressionStorageAdapter satisfies expression.StorageProvider by
// mapping FormSummary → expression.Record. Storage already harvests
// ExpressionItems on the list path; the adapter just renames the
// field and drops Meta (the engine doesn't need it).
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
// Manager.EvaluateListOne. Missing file → empty Record (matches
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
