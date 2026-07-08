package wiki

import (
	"context"

	"github.com/petervdpas/formidable2/internal/modules/bundle"
)

// ExportMeta is the author-supplied descriptor for a bundle, carried into the
// cleartext manifest so the Viewer can show what the pack is before unlocking.
// It holds no key material; the password does the protecting.
type ExportMeta struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Created     string `json:"created"`
	Kind        string `json:"kind"`
}

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

// ExportPack builds the offline-wiki bundle, wraps it as a branded .bundle
// (manifest + payload, sealed with password when non-empty), and writes it to
// path. selections maps a template filename to the deck values to include
// (empty = all decks for a presentation; ignored for a document). Returns the
// stems skipped (a template that failed to load, or a presentation with no
// exportable decks).
func (s *ExportService) ExportPack(selections map[string][]string, path string, password string, meta ExportMeta) ([]string, error) {
	packed, skipped, err := s.h.ExportPack(context.Background(), selections, password, bundle.Manifest{
		Title:       meta.Title,
		Description: meta.Description,
		Author:      meta.Author,
		Created:     meta.Created,
		Kind:        meta.Kind,
	})
	if err != nil {
		return nil, err
	}
	if err := s.saver.SaveBytes(path, packed); err != nil {
		return nil, err
	}
	return skipped, nil
}

// ResolveDependencies expands the given template picks into the full set the
// bundle needs (the picks plus every template they link to, transitively) so the
// frontend can auto-toggle the related templates on and explain why. The backend
// also applies this expansion at export time (ExportPack), so a bundle is
// self-contained even if the caller skips this call; this just surfaces it to the
// UI ahead of the export.
func (s *ExportService) ResolveDependencies(selected []string) (DependencyResult, error) {
	return s.h.Dependencies(selected)
}
