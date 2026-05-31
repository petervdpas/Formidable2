// Package logging is the Wails-facing facade over internal/log. It
// exposes the in-memory ring buffer (Recent) and the on-disk log file
// (ReadFile) so the Information->Logging panel can render a live tail
// and the raw text dump.
//
// WriteFromFrontend re-publishes SPA console.* calls through the same
// slog pipeline as backend code: frontend lines land in formidable.log
// and the live tail, tagged source=frontend.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	applog "github.com/petervdpas/formidable2/internal/log"
)

type Manager struct {
	bc      *applog.Broadcaster
	logPath string
	log     *slog.Logger
}

// NewManager wraps the broadcaster, log file path, and logger used by
// WriteFromFrontend. Any argument may be nil or empty: Recent, ReadFile,
// and WriteFromFrontend degrade to no-ops so the UI renders empty states.
func NewManager(bc *applog.Broadcaster, logPath string, log *slog.Logger) *Manager {
	return &Manager{bc: bc, logPath: logPath, log: log}
}

// Recent returns up to n entries from the broadcaster's ring buffer.
// n<=0 returns whatever is currently buffered.
func (m *Manager) Recent(n int) []applog.Entry {
	if m.bc == nil {
		return []applog.Entry{}
	}
	return m.bc.Recent(n)
}

// ReadFile returns the contents of the on-disk log file, or ("", nil)
// when file logging is disabled or the file doesn't exist yet so the UI
// shows "(empty)" without an error.
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

// LogPath returns the resolved log file path (empty when file logging is off).
func (m *Manager) LogPath() string { return m.logPath }

// WriteFromFrontend funnels a frontend console.* call through the app's
// logger so the line appears in both the live tail and formidable.log.
// source="frontend" is always stamped and cannot be overridden; it is
// the only way the UI distinguishes frontend from backend records.
// Fire-and-forget: empty messages, unknown levels, or a nil logger
// short-circuit silently.
func (m *Manager) WriteFromFrontend(level, msg string, fields map[string]any) {
	if m == nil || m.log == nil {
		return
	}
	if strings.TrimSpace(msg) == "" {
		return
	}
	lvl := parseFrontendLevel(level)

	attrs := make([]any, 0, 2+2*len(fields))
	for k, v := range fields {
		if k == "source" {
			continue
		}
		attrs = append(attrs, k, v)
	}
	attrs = append(attrs, "source", "frontend")

	m.log.Log(context.Background(), lvl, msg, attrs...)
}

func parseFrontendLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}
