package monitor

// Service is the Wails-bound surface over Manager. The frontend calls
// these methods directly (not via /api/monitor/*) so the Monitoring
// page works regardless of whether the loopback HTTP server is on.
//
// External API consumers reach the same Manager through NewHandler
// - both transports share the registered Sources and the Query/Result
// shapes.
type Service struct{ m *Manager }

// NewService wraps a Manager. Construct one Manager, register Sources
// against it, then build the Service for Wails and the Handler for
// HTTP from that single instance.
func NewService(m *Manager) *Service { return &Service{m: m} }

// Run executes a query. The Query JSON shape matches what the HTTP
// /api/monitor/query endpoint accepts.
func (s *Service) Run(q Query) (*Result, error) { return s.m.Run(q) }

// ListSources returns descriptors for every registered Source - used
// by query-builder UIs to render filter and group-by pickers without
// hard-coding dim names.
func (s *Service) ListSources() []SourceInfo { return s.m.ListSources() }
