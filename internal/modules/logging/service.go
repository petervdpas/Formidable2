package logging

import applog "github.com/petervdpas/formidable2/internal/log"

// Service is the Wails binding surface. Frontend uses the Logging
// service to fetch the in-memory tail and the raw log file.
type Service struct {
	mgr *Manager
}

func NewService(mgr *Manager) *Service { return &Service{mgr: mgr} }

// Recent — up to `n` of the most-recent buffered entries. n<=0 returns
// whatever the ring currently holds.
func (s *Service) Recent(n int) []applog.Entry { return s.mgr.Recent(n) }

// ReadFile — full contents of formidable.log (or "" when file logging
// is off / the file doesn't exist yet).
func (s *Service) ReadFile() (string, error) { return s.mgr.ReadFile() }

// LogPath — resolved on-disk log path; "" means file logging is off.
func (s *Service) LogPath() string { return s.mgr.LogPath() }

// LogFromFrontend re-publishes a SPA console.* call through the app's
// slog pipeline so frontend lines appear in both formidable.log and
// the live tail. Always tagged with source=frontend.
//
// Level is one of "debug" | "info" | "warn" | "error"; anything else
// (or empty) falls back to "info". Empty/whitespace msgs are dropped.
// Always returns nil so the frontend wrapper never has to handle a
// reject path — losing a log line is preferable to looping.
func (s *Service) LogFromFrontend(level, msg string, fields map[string]any) error {
	s.mgr.WriteFromFrontend(level, msg, fields)
	return nil
}
