package csv

// Service is the api layer over Manager. Methods map to the old Electron
// `window.api.csv.*` IPC group:
//   - csv-preview     → Preview
//   - csv-write       → Write
//
// `csv-import-row` is intentionally NOT in this module — the Electron
// app already routed it to formManager.saveForm. Storage import (F-302)
// will own that route.
//
// Wails-only: HTTP export endpoints (Epic 8 collections API) call into
// this manager directly; no `handlers.go` lives here.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

func (s *Service) Preview(filePath, delimiter string) (PreviewResult, error) {
	return s.m.Preview(filePath, delimiter)
}

func (s *Service) Write(filePath string, rows [][]string, delimiter string) WriteResult {
	return s.m.Write(filePath, rows, delimiter)
}
