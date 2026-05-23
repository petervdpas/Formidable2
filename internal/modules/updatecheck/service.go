package updatecheck

import (
	"context"
	"errors"
)

// Service is the Wails-facing surface. GetStatus reads the cached
// verdict (populated by the startup probe); CheckNow re-probes on
// demand; OpenLatest opens the release page via the injected platform
// browser opener (same shim about.Service uses).
type Service struct {
	m           *Manager
	openBrowser func(string) error
}

func NewService(m *Manager, openBrowser func(string) error) *Service {
	return &Service{m: m, openBrowser: openBrowser}
}

func (s *Service) GetStatus() Status {
	return s.m.GetStatus()
}

// CheckNow forces a fresh probe and returns the resulting status. The
// error is swallowed: callers get a Status with Checked=false on
// failure, never an exception.
func (s *Service) CheckNow() Status {
	st, _ := s.m.Refresh(context.Background())
	return st
}

// OpenLatest opens the latest release page in the default browser.
func (s *Service) OpenLatest() error {
	url := s.m.GetStatus().URL
	if url == "" {
		return errors.New("updatecheck: no release URL known")
	}
	if s.openBrowser == nil {
		return errors.New("updatecheck: OpenLatest not supported on this build")
	}
	return s.openBrowser(url)
}
