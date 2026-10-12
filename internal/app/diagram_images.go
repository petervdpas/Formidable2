package app

import (
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// diagramImageSync keeps diagram image files in step with a changed template:
// it runs the image save path for every record (a diagram field was added, its
// table changed) and prunes files whose record or field is gone. It must run
// after tableColumnMigrator, which realigns the rows the diagrams read.
type diagramImageSync struct{ sto *storage.Manager }

func (s diagramImageSync) OnTemplateUpdated(name string, _, _ *template.Template) error {
	if s.sto == nil {
		return nil
	}
	return s.sto.SyncDiagramImages(name)
}
