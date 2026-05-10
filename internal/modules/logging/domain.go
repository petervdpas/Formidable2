// Package logging is the Wails-facing facade over internal/log.
// It exposes the in-memory ring buffer (Recent) and the on-disk log
// file contents (ReadFile) so the Information→Logging panel can
// render both a live tail and the raw text dump.
package logging

import (
	"fmt"
	"os"

	applog "github.com/petervdpas/formidable2/internal/log"
)

type Manager struct {
	bc      *applog.Broadcaster
	logPath string
}

// NewManager wraps the broadcaster and remembers the log file path.
// Either argument may be empty/nil — Recent returns [] and ReadFile
// returns "" without error in that case so the UI can render empty
// states without special-casing.
func NewManager(bc *applog.Broadcaster, logPath string) *Manager {
	return &Manager{bc: bc, logPath: logPath}
}

// Recent returns up to n entries from the broadcaster's ring buffer.
// n<=0 returns whatever is currently buffered.
func (m *Manager) Recent(n int) []applog.Entry {
	if m.bc == nil {
		return []applog.Entry{}
	}
	return m.bc.Recent(n)
}

// ReadFile returns the contents of the on-disk log file. Returns
// ("", nil) when file logging is disabled or the file doesn't exist
// yet so the UI can show "(empty)" without surfacing an error.
func (m *Manager) ReadFile() (string, error) {
	if m.logPath == "" {
		return "", nil
	}
	body, err := os.ReadFile(m.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read log file %q: %w", m.logPath, err)
	}
	return string(body), nil
}

// LogPath returns the resolved log file path (empty when file logging
// is off). Useful for the UI footer / debug.
func (m *Manager) LogPath() string { return m.logPath }
