package journal

// Service is the Wails read surface over Manager (configure/record/sync stay internal).
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Pending returns the pending changes for the backend (empty for none/unknown).
func (s *Service) Pending(backend string) PendingResult { return s.m.Pending(backend) }

// Cursor returns the per-backend sync watermarks.
func (s *Service) Cursor() CursorMap { return s.m.ReadCursor() }

// RecentEntries returns the recent log entries, newest first (limit <= 0 returns all).
func (s *Service) RecentEntries(limit int) []Entry { return s.m.RecentEntries(limit) }

// ListSyncBackends returns the canonical sync-backend ids in display order.
func (s *Service) ListSyncBackends() []string {
	out := make([]string, len(orderedSyncBackends))
	copy(out, orderedSyncBackends)
	return out
}
