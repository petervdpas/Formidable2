package index

import (
	"context"

	"github.com/petervdpas/formidable2/internal/optrack"
)

// Service is the thin Wails facade over the index's on-demand
// maintenance actions. The index is otherwise driven internally (save/
// delete event hooks and the startup RescanAll); the only operation the
// frontend needs to invoke directly is a force-reindex of one template's
// collection, exposed here.
type Service struct {
	h   *EventHandler
	ops *optrack.Registry
}

func NewService(h *EventHandler) *Service { return &Service{h: h} }

// AttachOps installs the shared op registry so a reindex registers its state
// (guarding "cannot run twice" per template and letting the frontend resume on reload).
func AttachOps(s *Service, ops *optrack.Registry) {
	if s == nil {
		return
	}
	s.ops = ops
}

// RescanTemplate force-reindexes the given template's collection from
// disk (see EventHandler.RescanTemplate). Wails supplies no request
// context here, so a background context is used. Tracked and guarded per
// template, so the same collection cannot reindex twice at once.
func (s *Service) RescanTemplate(templateFilename string) error {
	_, release, err := optrack.Guard(s.ops, "index:rescan:"+templateFilename)
	if err != nil {
		return err
	}
	defer release()
	return s.h.RescanTemplate(context.Background(), templateFilename)
}
