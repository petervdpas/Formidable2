package log

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"DEBUG":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
	}
	for in, want := range cases {
		got, ok := parseLevel(in)
		if !ok || got != want {
			t.Errorf("parseLevel(%q) = (%v,%v), want (%v,true)", in, got, ok, want)
		}
	}
	if _, ok := parseLevel("nonsense"); ok {
		t.Error("parseLevel(nonsense) should fail")
	}
	if _, ok := parseLevel(""); ok {
		t.Error("parseLevel(empty) should fail")
	}
}

func TestPickLevel_FromOptions(t *testing.T) {
	t.Setenv(envLogLevel, "")
	if got := pickLevel("warn"); got != slog.LevelWarn {
		t.Errorf("pickLevel(warn) = %v", got)
	}
}

func TestPickLevel_FromEnv(t *testing.T) {
	t.Setenv(envLogLevel, "debug")
	if got := pickLevel(""); got != slog.LevelDebug {
		t.Errorf("pickLevel(env=debug) = %v", got)
	}
}

func TestPickLevel_FallbackInfo(t *testing.T) {
	t.Setenv(envLogLevel, "")
	if got := pickLevel(""); got != slog.LevelInfo {
		t.Errorf("pickLevel default = %v, want INFO", got)
	}
}

func TestPickUseFile(t *testing.T) {
	t.Setenv(envLogFile, "")
	if !pickUseFile(nil) {
		t.Error("default should be true")
	}

	t.Setenv(envLogFile, "0")
	if pickUseFile(nil) {
		t.Error("env=0 should disable")
	}
	t.Setenv(envLogFile, "false")
	if pickUseFile(nil) {
		t.Error("env=false should disable")
	}
	t.Setenv(envLogFile, "off")
	if pickUseFile(nil) {
		t.Error("env=off should disable")
	}

	// explicit override beats env
	t.Setenv(envLogFile, "0")
	if !pickUseFile(boolPtr(true)) {
		t.Error("explicit true should beat env=0")
	}
}

func TestNew_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	l := New(Options{AppRoot: dir, Level: "debug"})
	l.Info("hello world", "key", "value")

	body, err := os.ReadFile(filepath.Join(dir, defaultFileName))
	if err != nil {
		t.Fatalf("expected log file: %v", err)
	}
	if !strings.Contains(string(body), "hello world") {
		t.Errorf("log file missing message: %q", body)
	}
	if !strings.Contains(string(body), "key=value") {
		t.Errorf("log file missing attr: %q", body)
	}
}

func TestNew_FileDisabled(t *testing.T) {
	dir := t.TempDir()
	l := New(Options{AppRoot: dir, UseFile: boolPtr(false)})
	l.Info("no file please")

	if _, err := os.Stat(filepath.Join(dir, defaultFileName)); !os.IsNotExist(err) {
		t.Errorf("expected no log file, got err=%v", err)
	}
}

func TestNew_RespectsLevel(t *testing.T) {
	dir := t.TempDir()
	l := New(Options{AppRoot: dir, Level: "warn"})
	l.Debug("hidden")
	l.Info("hidden too")
	l.Warn("visible warn")

	body, _ := os.ReadFile(filepath.Join(dir, defaultFileName))
	s := string(body)
	if strings.Contains(s, "hidden") {
		t.Errorf("debug/info entries should be filtered: %q", s)
	}
	if !strings.Contains(s, "visible warn") {
		t.Errorf("warn entry missing: %q", s)
	}
}

func TestNew_OpenFailureFallsBackToStderr(t *testing.T) {
	// Force open to fail; logger should still be usable.
	prev := openAppendFile
	t.Cleanup(func() { openAppendFile = prev })
	openAppendFile = func(string) (*os.File, error) { return nil, os.ErrPermission }

	l := New(Options{AppRoot: t.TempDir()})
	l.Info("still alive") // must not panic
}
