// Package log builds the application's structured logger.
//
// The logger always writes to stderr and (by default) tees to
// <AppRoot>/formidable.log. Level can be set programmatically via
// Options.Level or via the FORMIDABLE_LOG_LEVEL environment variable.
// File output can be disabled by setting FORMIDABLE_LOG_FILE to "0",
// "false", or "off".
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

// New constructs a logger per the options. Errors opening the log file are
// silently swallowed and the logger falls back to stderr-only — file logging
// must never prevent the application from starting.
func New(opts Options) *slog.Logger {
	level := pickLevel(opts.Level)
	useFile := pickUseFile(opts.UseFile)

	var writer io.Writer = os.Stderr
	if useFile && opts.AppRoot != "" {
		fileName := opts.FileName
		if fileName == "" {
			fileName = defaultFileName
		}
		path := filepath.Join(opts.AppRoot, fileName)
		if f, err := openAppendFile(path); err == nil {
			writer = io.MultiWriter(os.Stderr, f)
		}
	}

	handlerOpts := &slog.HandlerOptions{Level: level}
	if opts.JSON {
		return slog.New(slog.NewJSONHandler(writer, handlerOpts))
	}
	return slog.New(slog.NewTextHandler(writer, handlerOpts))
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

// openAppendFile is a var so tests can override file-open behaviour without
// touching the filesystem. Uses an internal mutex to avoid the rare race of
// two New() calls opening the same path concurrently in tests.
var (
	openMu        sync.Mutex
	openAppendFile = func(path string) (*os.File, error) {
		openMu.Lock()
		defer openMu.Unlock()
		return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	}
)
