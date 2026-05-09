package app

import (
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// expressionTemplateAdapter satisfies expression.TemplateProvider so
// the engine module never needs to import template directly. Returns
// the sidebar expression and the list of expression-flagged field
// keys — that field list backs the engine's narrowContext defence.
type expressionTemplateAdapter struct {
	tpl *template.Manager
}

// LookupSidebar reads the template's sidebar_expression and the
// names of every field flagged expression_item: true. Empty
// expression maps to ErrNoExpression at the engine layer; the
// adapter itself returns ("", nil, nil) so the contract stays
// explicit (no error means "loaded fine, but configured empty").
func (a expressionTemplateAdapter) LookupSidebar(name string) (string, []string, error) {
	t, err := a.tpl.LoadTemplate(name)
	if err != nil {
		return "", nil, err
	}
	keys := make([]string, 0, len(t.Fields))
	for _, f := range t.Fields {
		if f.ExpressionItem {
			keys = append(keys, f.Key)
		}
	}
	return t.SidebarExpression, keys, nil
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
