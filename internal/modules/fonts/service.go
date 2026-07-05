package fonts

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Service is the Wails-bound facade for the fonts resource. Vue calls these from
// the Information -> Fonts panel and the slide font picker.
type Service struct{ m *Manager }

// NewService wraps a Manager for Wails binding.
func NewService(m *Manager) *Service { return &Service{m: m} }

// ListFonts returns a descriptor per font under <AppRoot>/fonts/ (IsSeed flags a
// factory font).
func (s *Service) ListFonts() ([]FontInfo, error) { return s.m.List() }

// SaveFont decodes a base64 (or data-URI) body and persists it under
// <AppRoot>/fonts/<name>. Extension/traversal validation happens in the Manager.
func (s *Service) SaveFont(name, base64Data string) error {
	raw, err := decodeBase64Body(base64Data)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFont, err)
	}
	return s.m.Save(name, raw)
}

// LoadFont returns one font's bytes, base64-encoded for a JSON round-trip.
func (s *Service) LoadFont(name string) (string, error) {
	raw, err := s.m.Load(name)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// DeleteFont removes a font. A factory font reappears via RestoreDefaultFonts.
func (s *Service) DeleteFont(name string) error { return s.m.Delete(name) }

// RestoreDefaultFonts rewrites any factory (seed) font the user deleted.
func (s *Service) RestoreDefaultFonts() error { return s.m.Scaffold() }

// FontFaceCSS returns @font-face rules (fonts inlined as data: URIs) so the slide
// canvas editor can inject uploaded fonts and preview them live.
func (s *Service) FontFaceCSS() (string, error) { return s.m.FontFaceCSS() }

// decodeBase64Body accepts a bare base64 string or a data-URI (data:...;base64,…)
// and tolerates both standard and raw (unpadded) encodings.
func decodeBase64Body(data string) ([]byte, error) {
	if i := strings.Index(data, ";base64,"); i >= 0 {
		data = data[i+len(";base64,"):]
	}
	data = strings.TrimSpace(data)
	if raw, err := base64.StdEncoding.DecodeString(data); err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(data)
}
