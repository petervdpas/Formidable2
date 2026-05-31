package app

import (
	"github.com/petervdpas/formidable2/internal/modules/query"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// queryLoaderAdapter bridges template + storage into the narrow query.Loader.
// Query reads structured form data directly (not the index), so any field is
// queryable and table rows arrive row-aligned. Lives in the composition root:
// query owns no opinion about storage, storage none about query.
type queryLoaderAdapter struct {
	tpl *template.Manager
	sto *storage.Manager
}

func newQueryLoaderAdapter(tpl *template.Manager, sto *storage.Manager) *queryLoaderAdapter {
	return &queryLoaderAdapter{tpl: tpl, sto: sto}
}

func (a *queryLoaderAdapter) Template(name string) (*template.Template, error) {
	return a.tpl.LoadTemplate(name)
}

// Forms loads every form as raw data + set facets. A malformed/missing form
// (LoadForm returns nil) is skipped rather than failing the whole query.
func (a *queryLoaderAdapter) Forms(name string) ([]query.FormData, error) {
	files, err := a.sto.ListForms(name)
	if err != nil {
		return nil, err
	}
	out := make([]query.FormData, 0, len(files))
	for _, file := range files {
		f := a.sto.LoadForm(name, file)
		if f == nil {
			continue
		}
		var facets map[string]string
		if len(f.Meta.Facets) > 0 {
			facets = make(map[string]string, len(f.Meta.Facets))
			for k, st := range f.Meta.Facets {
				if st.Set {
					facets[k] = st.Selected
				}
			}
		}
		out = append(out, query.FormData{Filename: file, Data: f.Data, Facets: facets})
	}
	return out, nil
}
