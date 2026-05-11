package app

import (
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
// the index — so the adapter lives in the composition root.
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
// nil for both "missing" and "malformed" — the index treats either as
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

// stemOf strips ".yaml" so "basic.yaml" → "basic" — the per-template
// storage subdirectory name that storage.Manager uses internally.
func stemOf(templateFilename string) string {
	if ext := filepath.Ext(templateFilename); ext != "" {
		return templateFilename[:len(templateFilename)-len(ext)]
	}
	return templateFilename
}
