package app

import (
	"github.com/petervdpas/formidable2/internal/modules/csv"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// csvFormsAdapter satisfies csv.formsSource so the csv module lists and reads
// forms without importing storage directly. Export() is the only caller.
type csvFormsAdapter struct {
	sto *storage.Manager
}

func (a *csvFormsAdapter) ListForms(tpl string) ([]string, error) {
	return a.sto.ListForms(tpl)
}

// LoadFormData returns the .data block of a stored form, or nil when the file
// is missing or unreadable. The exporter treats nil as "skip this entry" so a
// transient read failure doesn't blow up the whole job.
func (a *csvFormsAdapter) LoadFormData(tpl, datafile string) map[string]any {
	f := a.sto.LoadForm(tpl, datafile)
	if f == nil {
		return nil
	}
	return f.Data
}

// csvTemplateAdapter satisfies csv.templateSource: loads a template and projects
// its fields into FieldSpec so the exporter owns excluded types, alignability,
// and the dotted-key contract without importing template's full Field type.
type csvTemplateAdapter struct {
	tpl *template.Manager
}

func (a *csvTemplateAdapter) Fields(name string) ([]csv.FieldSpec, error) {
	t, err := a.tpl.LoadTemplate(name)
	if err != nil {
		return nil, err
	}
	out := make([]csv.FieldSpec, 0, len(t.Fields))
	for _, f := range t.Fields {
		out = append(out, csv.FieldSpec{
			Key:     f.Key,
			Type:    f.Type,
			Label:   f.Label,
			Options: f.Options,
		})
	}
	return out, nil
}
