package journal

// Service is the api layer over Manager. Methods map 1:1 with the old
// Electron `window.api.journal.*` IPC group (camelCase'd → PascalCase).
//
// Wails-only: configure / record / sync stay internal - only the
// pollers want to read state from the frontend.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Pending returns the pending changes for the requested backend.
// Empty backend or "none" returns an empty result.
func (s *Service) Pending(backend string) PendingResult { return s.m.Pending(backend) }

// Cursor returns the per-backend sync watermarks.
func (s *Service) Cursor() CursorMap { return s.m.ReadCursor() }

// RecentEntries exposes the journal's recent log entries (newest
// first) to the frontend journal-feed view. limit <= 0 returns all.
func (s *Service) RecentEntries(limit int) []Entry { return s.m.RecentEntries(limit) }

// ListSyncBackends returns the canonical sync-backend ids in display
// order. The collaboration sidebar (Vue) and the monitor "pending per
// backend" tile both read this - there is no other place where the
// list of backends should be enumerated on the frontend.
func (s *Service) ListSyncBackends() []string {
	out := make([]string, len(orderedSyncBackends))
	copy(out, orderedSyncBackends)
	return out
}
