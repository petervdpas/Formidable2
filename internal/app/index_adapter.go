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

// indexLoaderAdapter bridges template + storage managers into the narrow
// interfaces the index module needs. Lives in the composition root because the
// index must not import template/storage and they must not know about the index.
//
// Load* methods stat the on-disk file to attach a fresh mtime, so RescanAll's
// diff settles (no spurious "Changed" verdicts for rows we just wrote).
type indexLoaderAdapter struct {
	tpl *template.Manager
	sto *storage.Manager
}

func newIndexLoaderAdapter(tpl *template.Manager, sto *storage.Manager) *indexLoaderAdapter {
	return &indexLoaderAdapter{tpl: tpl, sto: sto}
}

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

// LoadForm: storage.Manager.LoadForm returns nil for both "missing" and
// "malformed"; the index treats either as a load failure and RescanAll skips
// the bad row while populating the rest.
func (a *indexLoaderAdapter) LoadForm(templateFilename, datafile string) (*index.FormRecord, error) {
	f := a.sto.LoadForm(templateFilename, datafile)
	if f == nil {
		return nil, fmt.Errorf("index loader: form %q/%q missing or unparseable", templateFilename, datafile)
	}
	stem := stemOf(templateFilename)
	mtime := statMtimeNanos(filepath.Join(a.sto.StorageDir(), stem, datafile))
	return &index.FormRecord{Form: f, Mtime: mtime}, nil
}

// statMtimeNanos returns 0 on stat failure. Zero is a sentinel the diff treats
// as "different from a real mtime", so the next RescanAll picks up real values.
func statMtimeNanos(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

// stemOf strips ".yaml" ("basic.yaml" to "basic"), the per-template storage
// subdirectory name storage.Manager uses internally.
func stemOf(templateFilename string) string {
	if ext := filepath.Ext(templateFilename); ext != "" {
		return templateFilename[:len(templateFilename)-len(ext)]
	}
	return templateFilename
}

// indexFormReader bridges *index.Manager into storage.FormReader, so
// storage.ExtendedListForms pulls summaries from the SQLite index instead of
// walking disk per record. Composition root: storage and index hold no opinion
// about each other.
type indexFormReader struct {
	idx *index.Manager
}

func newIndexFormReader(idx *index.Manager) *indexFormReader {
	return &indexFormReader{idx: idx}
}

// ListSummaries orders by filename ascending to match the original
// os.ReadDir-based disk path (no observable change for the studio sidebar).
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

// SearchSummaries orders by FTS5 relevance (SearchForms' own ranking), not
// filename, so the most relevant matches lead.
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
			// Wire shape is stem-keyed ("recipes") for FormSummary parity, even
			// though the index FK uses the full filename ("recipes.yaml") for joins.
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
