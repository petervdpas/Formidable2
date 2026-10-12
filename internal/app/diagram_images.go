package app

import (
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// diagramImagePrune removes generated diagram image files that no longer belong
// to a record + diagram field after a template change (e.g. the diagram field
// was removed). Drawing the files is the render step's job, not this one's.
type diagramImagePrune struct{ sto *storage.Manager }

func (p diagramImagePrune) OnTemplateUpdated(name string, _, _ *template.Template) error {
	if p.sto == nil {
		return nil
	}
	return p.sto.PruneDiagramImages(name)
}
