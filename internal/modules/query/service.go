package query

import "github.com/petervdpas/formidable2/internal/modules/index"

// Service is the Wails-facing layer over Manager. The studio query panel
// calls Run; the REST endpoint calls Manager.Run directly (it holds the
// manager, not the service).
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Run executes a query Spec and returns the typed Result. Errors
// (bad template, out-of-range column, invalid filter op) surface as the
// returned error so the frontend toast shows the backend message.
func (s *Service) Run(spec Spec) (Result, error) {
	return s.m.Run(spec)
}

// FilterOps returns the comparison operators a filter may use, in display
// order. Backend-owned (sourced from the datacore) so the query panel's
// operator picker can't drift from what the engine accepts.
func (s *Service) FilterOps() []string {
	return index.FilterOps
}
