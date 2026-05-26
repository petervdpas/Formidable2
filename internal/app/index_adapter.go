package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// indexLoaderAdapter bridges the existing template + storage managers
// into the narrow interfaces the index module needs. The index can't
// import template/storage directly without bringing in domain detail
// it doesn't care about, and template/storage shouldn't know about
// the index - so the adapter lives in the composition root.
//
// `Load*` methods stat the on-disk file to attach a fresh mtime/size
// to the record. This is what lets RescanAll's diff settle on
// a stable state on the next call (no spurious "Changed" verdicts for
// rows we just wrote).
type indexLoaderAdapter struct {
	tpl *template.Manager
	sto *storage.Manager
}

func newIndexLoaderAdapter(tpl *template.Manager, sto *storage.Manager) *indexLoaderAdapter {
	return &indexLoaderAdapter{tpl: tpl, sto: sto}
}

// LoadTemplate satisfies index.TemplateLoader.
func (a *indexLoaderAdapter) LoadTemplate(filename string) (*index.TemplateRecord, error) {
	t, err := a.tpl.LoadTemplate(filename)
	if err != nil {
		return nil, fmt.Errorf("index loader: load template %q: %w", filename, err)
	}
	if t == nil {
		return nil, fmt.Errorf("index loader: template %q not found", filename)
	}
	mtime := statMtimeNanos(filepath.Join(a.tpl.TemplatesDir(), filename))
	return &index.TemplateRecord{Template: t, Mtime: mtime}, nil
}

// LoadForm satisfies index.FormStore. storage.Manager.LoadForm returns
// nil for both "missing" and "malformed" - the index treats either as
// a load failure and the caller (RescanAll) skips the bad row but
// keeps populating the rest.
func (a *indexLoaderAdapter) LoadForm(templateFilename, datafile string) (*index.FormRecord, error) {
	f := a.sto.LoadForm(templateFilename, datafile)
	if f == nil {
		return nil, fmt.Errorf("index loader: form %q/%q missing or unparseable", templateFilename, datafile)
	}
	stem := stemOf(templateFilename)
	mtime := statMtimeNanos(filepath.Join(a.sto.StorageDir(), stem, datafile))
	return &index.FormRecord{Form: f, Mtime: mtime}, nil
}

// statMtimeNanos returns 0 on stat failure (file missing, permission
// denied, etc.). Zero is a sentinel the diff treats as "different from
// a real mtime", so the next RescanAll will pick up real values.
func statMtimeNanos(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

// stemOf strips ".yaml" so "basic.yaml" → "basic" - the per-template
// storage subdirectory name that storage.Manager uses internally.
func stemOf(templateFilename string) string {
	if ext := filepath.Ext(templateFilename); ext != "" {
		return templateFilename[:len(templateFilename)-len(ext)]
	}
	return templateFilename
}

// indexFormReader bridges *index.Manager into the storage.FormReader
// surface, so storage.ExtendedListForms can pull summaries straight
// from the SQLite index instead of walking disk per record.
//
// Stays in the composition root for the same reason as the loader
// adapter: storage owns no opinion about the index, and the index
// owns no opinion about FormSummary.
type indexFormReader struct {
	idx *index.Manager
}

func newIndexFormReader(idx *index.Manager) *indexFormReader {
	return &indexFormReader{idx: idx}
}

// ListSummaries satisfies storage.FormReader. Orders by filename
// ascending so the result matches what the original os.ReadDir-based
// disk path returned - no observable change for the studio sidebar.
func (r *indexFormReader) ListSummaries(templateFilename string) ([]storage.FormSummary, error) {
	rows, err := r.idx.ListForms(templateFilename, index.QueryOpts{OrderBy: "filename_asc"})
	if err != nil {
		return nil, err
	}
	out := make([]storage.FormSummary, 0, len(rows))
	for _, fr := range rows {
		out = append(out, formRowToSummary(fr))
	}
	return out, nil
}

// SearchSummaries satisfies storage.FormReader. Orders by FTS5
// relevance (SearchForms' own ranking), so the most relevant matches
// lead - search results aren't filename-sorted like the plain list.
func (r *indexFormReader) SearchSummaries(templateFilename, query string) ([]storage.FormSummary, error) {
	rows, err := r.idx.SearchForms(templateFilename, query, index.QueryOpts{})
	if err != nil {
		return nil, err
	}
	out := make([]storage.FormSummary, 0, len(rows))
	for _, fr := range rows {
		out = append(out, formRowToSummary(fr))
	}
	return out, nil
}

func formRowToSummary(r index.FormRow) storage.FormSummary {
	s := storage.FormSummary{
		Filename: r.Filename,
		Title:    r.Title,
		Meta: storage.FormMeta{
			ID: r.ID,
			// Storage's disk path stores the template stem ("recipes"),
			// not the yaml filename ("recipes.yaml"). Match that so
			// FormSummary parity holds - the index FK uses the filename
			// for clean joins, but the wire shape stays stem-keyed.
			Template: stemOf(r.Template),
			Created: storage.AuditEntry{
				At:    r.Created,
				Name:  r.CreatedName,
				Email: r.CreatedEmail,
			},
			Updated: storage.AuditEntry{
				At:    r.Updated,
				Name:  r.UpdatedName,
				Email: r.UpdatedEmail,
			},
			Tags: append([]string(nil), r.Tags...),
		},
	}
	if len(r.Facets) > 0 {
		s.Meta.Facets = make(map[string]storage.FacetState, len(r.Facets))
		for _, f := range r.Facets {
			s.Meta.Facets[f.Key] = storage.FacetState{Set: f.Set, Selected: f.Selected}
		}
	}
	if r.ExpressionItems != "" {
		var ei map[string]any
		if err := json.Unmarshal([]byte(r.ExpressionItems), &ei); err == nil {
			s.ExpressionItems = ei
		}
	}
	if s.ExpressionItems == nil {
		s.ExpressionItems = map[string]any{}
	}
	return s
}
