package dataprovider

import (
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/render"
)

// Index is what dataprovider needs from the index module. We declare
// it as an interface so unit tests can supply a fake without spinning
// up SQLite, and so a future caching layer can wrap *index.Manager
// without touching dataprovider itself.
type Index interface {
	ListTemplates() ([]index.TemplateRow, error)
	ListForms(template string, opts index.QueryOpts) ([]index.FormRow, error)
	GetForm(template, datafile string) (*index.FormRow, bool, error)
	ListByTags(tags []string) ([]index.FormRow, error)
	Rev() (int64, error)
}

// Renderer is what dataprovider needs from the render module — just
// the (template, datafile) → markdown+HTML pair. Same fake-friendly
// motivation as Index.
type Renderer interface {
	RenderForm(templateName, datafile string) (*render.Result, error)
}

// Manager is the dataprovider's only stateful object. It holds the
// composed dependencies; instances are cheap to create.
type Manager struct {
	idx Index
	ren Renderer
}

// NewManager wires the facade. The composition root creates one per
// active profile (alongside the per-profile index.Manager).
func NewManager(idx Index, ren Renderer) *Manager {
	return &Manager{idx: idx, ren: ren}
}
