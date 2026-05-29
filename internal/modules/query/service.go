package query

// Service is the Wails-facing layer over Manager. The studio query panel
// calls Run; the REST endpoint calls Manager.Run directly (it holds the
// manager, not the service).
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Sources lists the queryable sources for a template (fields, table
// columns, facets) with their capabilities, for the query panel's column,
// filter, group and measure pickers. Backend-owned so the frontend never
// reimplements the source universe.
func (s *Service) Sources(template string) ([]SourceInfo, error) {
	return s.m.Sources(template)
}

// Run executes a query Spec and returns the typed Result. Errors
// (bad template, out-of-range column, invalid filter op) surface as the
// returned error so the frontend toast shows the backend message.
func (s *Service) Run(spec Spec) (Result, error) {
	return s.m.Run(spec)
}

// Explain returns a read-only SQL-shaped preview of a spec, rendered by
// the backend so it stays truthful to what the engine runs.
func (s *Service) Explain(spec Spec) (string, error) {
	return s.m.Explain(spec)
}

// FilterOps returns the comparison operators a filter may use, in display
// order. Backend-owned (sourced from the datacore) so the query panel's
// operator picker can't drift from what the engine accepts.
func (s *Service) FilterOps() []string {
	return FilterOps
}
