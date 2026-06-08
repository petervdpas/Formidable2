package dataprovider

import (
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/storage"
)

// Index is what dataprovider needs from the index module.
type Index interface {
	ListTemplates() ([]index.TemplateRow, error)
	ListForms(template string, opts index.QueryOpts) ([]index.FormRow, error)
	GetForm(template, datafile string) (*index.FormRow, bool, error)
	ListByTags(tags []string) ([]index.FormRow, error)
	FormsWithValueOp(template, fieldKey, op, value string) ([]string, error)
	Rev() (int64, error)
}

// Renderer is what dataprovider needs from the render module: the
// (template, datafile) to markdown+HTML pair.
type Renderer interface {
	RenderForm(templateName, datafile string) (*render.Result, error)
}

// Storage is what dataprovider needs to read raw form data. Returns nil
// for missing forms.
type Storage interface {
	LoadForm(template, datafile string) *storage.Form
}

// Manager is the dataprovider's only stateful object. It holds the
// composed dependencies; instances are cheap to create.
type Manager struct {
	idx Index
	ren Renderer
	sto Storage
}

// NewManager wires the facade. The composition root creates one per
// active profile (alongside the per-profile index.Manager). Storage
// may be nil when the caller doesn't need api-field reads (e.g. unit
// tests that only exercise the index/render paths).
func NewManager(idx Index, ren Renderer, sto Storage) *Manager {
	return &Manager{idx: idx, ren: ren, sto: sto}
}
