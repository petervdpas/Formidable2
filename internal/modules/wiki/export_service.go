package wiki

import "context"

// BytesSaver writes raw bytes to a path atomically; system.Manager satisfies it.
type BytesSaver interface {
	SaveBytes(path string, content []byte) error
}

// ExportService is the Wails surface for the offline-wiki bundle export. It
// holds the same Handler the HTTP server uses (so pages render identically) plus
// a byte-saver, keeping the binary zip off the Wails IPC boundary.
type ExportService struct {
	h     *Handler
	saver BytesSaver
}

// NewExportService wraps the wiki Handler and a byte-saver, panicking on nil so
// a composition-root bug surfaces at boot.
func NewExportService(h *Handler, saver BytesSaver) *ExportService {
	if h == nil || saver == nil {
		panic("wiki: NewExportService called with nil handler or saver")
	}
	return &ExportService{h: h, saver: saver}
}

// ExportBundle builds a self-contained offline-wiki zip and writes it to path.
// selections maps a template filename to the deck values to include (empty = all
// decks for a presentation; ignored for a document). Returns the stems skipped
// (a template that failed to load, or a presentation with no exportable decks).
func (s *ExportService) ExportBundle(selections map[string][]string, path string) ([]string, error) {
	res, err := s.h.ExportBundle(context.Background(), selections)
	if err != nil {
		return nil, err
	}
	if err := s.saver.SaveBytes(path, res.Zip); err != nil {
		return nil, err
	}
	return res.Skipped, nil
}
