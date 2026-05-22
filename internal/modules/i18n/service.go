package i18n

// Service is the Wails-facing wrapper. The frontend boots, calls
// LoadBundle for the active locale, and feeds the result into vue-i18n.
// Locale changes drive a fresh LoadBundle - there is no per-key RPC.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// LoadBundle returns the full translation map for locale. Frontend
// merges this into vue-i18n's `messages` for the same locale id.
func (s *Service) LoadBundle(locale string) (map[string]any, error) {
	return s.m.LoadBundle(locale)
}

// AvailableLocales returns the sorted list of locale ids the binary
// ships. Used by the runtime bootstrap; the language picker uses
// ListLocales (which carries the endonym alongside the code).
func (s *Service) AvailableLocales() []string {
	return s.m.AvailableLocales()
}

// ListLocales returns sorted LocaleDescriptors (code + endonym) for
// every locale the binary ships. Replaces the hardcoded language
// array in the Settings → General language picker - endonyms come
// from each locale's `language.endonym` bundle key, so adding a new
// locale just means adding the file (no central registry to update).
func (s *Service) ListLocales() []LocaleDescriptor {
	return s.m.ListLocales()
}

// DefaultLocale returns the canonical fallback locale id.
func (s *Service) DefaultLocale() string {
	return s.m.DefaultLocale()
}
