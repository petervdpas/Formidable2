// Package log builds the application's structured logger.
//
// The logger always writes to stderr and (by default) tees to
// <AppRoot>/formidable.log. The log file is truncated on every
// New() call, so each app start begins with a fresh log. Level can
// be set programmatically via Options.Level or via the
// FORMIDABLE_LOG_LEVEL environment variable. File output can be
// disabled by setting FORMIDABLE_LOG_FILE to "0", "false", or "off".
package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultFileName = "formidable.log"
	envLogLevel     = "FORMIDABLE_LOG_LEVEL"
	envLogFile      = "FORMIDABLE_LOG_FILE"
)

type Options struct {
	// AppRoot is the directory that holds formidable.log when file logging is on.
	// Empty disables file output regardless of UseFile.
	AppRoot string

	// FileName overrides "formidable.log".
	FileName string

	// Level overrides FORMIDABLE_LOG_LEVEL. Accepts debug/info/warn/error.
	// Empty falls through to the env var, then to info.
	Level string

	// UseFile enables file output. Defaults to true. The FORMIDABLE_LOG_FILE
	// env var can force this off ("0", "false", "off").
	UseFile *bool

	// JSON switches to slog's JSONHandler. Default is TextHandler.
	JSON bool
}

// LogPath returns the resolved log file path for the given options,
// or "" when file output is disabled or AppRoot is empty.
func LogPath(opts Options) string {
	if !pickUseFile(opts.UseFile) || opts.AppRoot == "" {
		return ""
	}
	fileName := opts.FileName
	if fileName == "" {
		fileName = defaultFileName
	}
	return filepath.Join(opts.AppRoot, fileName)
}

// New constructs a logger + Broadcaster per the options. Errors
// opening the log file are silently swallowed and the logger falls
// back to stderr-only — file logging must never prevent the
// application from starting. The broadcaster is always returned and
// captures every record into its in-memory ring; pair it with
// Broadcaster.SetEmitter to fan records out to a UI transport.
func New(opts Options) (*slog.Logger, *Broadcaster) {
	level := pickLevel(opts.Level)
	useFile := pickUseFile(opts.UseFile)

	var writer io.Writer = os.Stderr
	if useFile && opts.AppRoot != "" {
		fileName := opts.FileName
		if fileName == "" {
			fileName = defaultFileName
		}
		path := filepath.Join(opts.AppRoot, fileName)
		if f, err := openLogFile(path); err == nil {
			writer = io.MultiWriter(os.Stderr, f)
		}
	}

	handlerOpts := &slog.HandlerOptions{Level: level}
	var textHandler slog.Handler
	if opts.JSON {
		textHandler = slog.NewJSONHandler(writer, handlerOpts)
	} else {
		textHandler = slog.NewTextHandler(writer, handlerOpts)
	}
	bc := NewBroadcaster(500)
	return slog.New(newMultiHandler(textHandler, bc.Handler())), bc
}

func pickLevel(explicit string) slog.Level {
	if lvl, ok := parseLevel(explicit); ok {
		return lvl
	}
	if lvl, ok := parseLevel(os.Getenv(envLogLevel)); ok {
		return lvl
	}
	return slog.LevelInfo
}

func parseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return 0, false
}

func pickUseFile(explicit *bool) bool {
	if explicit != nil {
		return *explicit
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envLogFile))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// openLogFile opens the log file for the current process, truncating
// any previous content so each app start gets a fresh log. Exposed as
// a var so tests can override file-open behaviour without touching the
// filesystem. The mutex avoids a rare race when two New() calls hit the
// same path concurrently (tests).
var (
	openMu      sync.Mutex
	openLogFile = func(path string) (*os.File, error) {
		openMu.Lock()
		defer openMu.Unlock()
		return os.OpenFile(path, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
	}
)
