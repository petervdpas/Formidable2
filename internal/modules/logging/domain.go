// Package logging is the Wails-facing facade over internal/log.
// It exposes the in-memory ring buffer (Recent) and the on-disk log
// file contents (ReadFile) so the Information→Logging panel can
// render both a live tail and the raw text dump.
//
// WriteFromFrontend lets the SPA re-publish console.* calls through
// the same slog pipeline as backend code — frontend lines land in
// formidable.log AND the live tail, tagged with source=frontend so
// they're distinguishable from backend records.
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

// NewManager wraps the broadcaster, remembers the log file path, and
// holds the *slog.Logger used by WriteFromFrontend. Any argument may
// be nil/empty — Recent/ReadFile/WriteFromFrontend all degrade to
// no-ops so the UI renders empty states without special-casing.
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

// WriteFromFrontend funnels a frontend console.* call through the
// app's *slog.Logger. The record flows to every handler the logger
// fans (text → stderr/file, broadcaster → ring + Wails event), so
// frontend lines appear in the live tail AND in formidable.log.
//
// source="frontend" is always stamped and cannot be overridden by the
// caller — the attribute is the only way the UI distinguishes
// frontend from backend records.
//
// Returns no error: this is fire-and-forget. Empty/whitespace
// messages, unknown levels, or a nil logger short-circuit silently
// so the frontend's console-wrapper never has to think about errors.
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
