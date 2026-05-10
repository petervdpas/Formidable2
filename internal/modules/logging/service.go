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
