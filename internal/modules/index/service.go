package index

import "context"

// Service is the thin Wails facade over the index's on-demand
// maintenance actions. The index is otherwise driven internally (save/
// delete event hooks and the startup RescanAll); the only operation the
// frontend needs to invoke directly is a force-reindex of one template's
// collection, exposed here.
type Service struct{ h *EventHandler }

func NewService(h *EventHandler) *Service { return &Service{h: h} }

// RescanTemplate force-reindexes the given template's collection from
// disk (see EventHandler.RescanTemplate). Wails supplies no request
// context here, so a background context is used.
func (s *Service) RescanTemplate(templateFilename string) error {
	return s.h.RescanTemplate(context.Background(), templateFilename)
}
