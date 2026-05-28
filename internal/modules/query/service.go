package query

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
