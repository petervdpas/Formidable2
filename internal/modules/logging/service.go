package logging

import applog "github.com/petervdpas/formidable2/internal/log"

// Service is the Wails binding surface for fetching the in-memory tail
// and the raw log file.
type Service struct {
	mgr *Manager
}

func NewService(mgr *Manager) *Service { return &Service{mgr: mgr} }

// Recent returns up to n of the most-recent buffered entries; n<=0
// returns whatever the ring currently holds.
func (s *Service) Recent(n int) []applog.Entry { return s.mgr.Recent(n) }

// ReadFile returns the full contents of formidable.log, or "" when file
// logging is off or the file doesn't exist yet.
func (s *Service) ReadFile() (string, error) { return s.mgr.ReadFile() }

// LogPath returns the resolved on-disk log path; "" means file logging is off.
func (s *Service) LogPath() string { return s.mgr.LogPath() }

// LogFromFrontend re-publishes a SPA console.* call through the app's
// slog pipeline so frontend lines appear in both formidable.log and the
// live tail, tagged source=frontend. Level is "debug"|"info"|"warn"|"error";
// anything else falls back to "info" and empty messages are dropped.
// Always returns nil so the frontend wrapper never has a reject path.
func (s *Service) LogFromFrontend(level, msg string, fields map[string]any) error {
	s.mgr.WriteFromFrontend(level, msg, fields)
	return nil
}
