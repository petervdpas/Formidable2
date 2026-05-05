package nav

// Service is the Wails-bound facade for nav. Vue calls these from the
// FormFieldLink component (bare-link click) and a future global click
// delegator on the rendered HTML preview.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// NavigateToFormidable resolves a formidable://… URL, validates that
// the template + datafile exist, persists the selection in config,
// and emits `nav:changed`. Vue toggles the active workspace in
// response.
func (s *Service) NavigateToFormidable(href string) (*Result, error) {
	return s.m.NavigateToFormidable(href)
}

// ResolveFormidable performs only parsing + validation. Useful for
// hover tooltips and the future internal HTTP server.
func (s *Service) ResolveFormidable(href string) (*Result, error) {
	return s.m.ResolveFormidable(href)
}
