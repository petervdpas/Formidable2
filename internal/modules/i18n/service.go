package i18n

// Service is the Wails-facing wrapper. The frontend boots, calls
// LoadBundle for the active locale, and feeds the result into vue-i18n.
// Locale changes drive a fresh LoadBundle — there is no per-key RPC.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// LoadBundle returns the full translation map for locale. Frontend
// merges this into vue-i18n's `messages` for the same locale id.
func (s *Service) LoadBundle(locale string) (map[string]any, error) {
	return s.m.LoadBundle(locale)
}

// AvailableLocales returns the sorted list of locale ids the binary
// ships. Used to populate the language picker in Settings.
func (s *Service) AvailableLocales() []string {
	return s.m.AvailableLocales()
}

// DefaultLocale returns the canonical fallback locale id.
func (s *Service) DefaultLocale() string {
	return s.m.DefaultLocale()
}
