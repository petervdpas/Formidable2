package monitor

// Service is the Wails-bound surface over Manager. The frontend calls
// these methods directly (not via /api/monitor/*) so the Monitoring
// page works regardless of whether the loopback HTTP server is on.
// External consumers reach the same Manager through NewHandler.
type Service struct{ m *Manager }

// NewService wraps a Manager.
func NewService(m *Manager) *Service { return &Service{m: m} }

// Run executes a query. The Query JSON shape matches the HTTP
// /api/monitor/query endpoint.
func (s *Service) Run(q Query) (*Result, error) { return s.m.Run(q) }

// ListSources returns descriptors for every registered Source.
func (s *Service) ListSources() []SourceInfo { return s.m.ListSources() }
