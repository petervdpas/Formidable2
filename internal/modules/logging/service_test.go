package logging

import (
	"os"
	"path/filepath"
	"testing"

	applog "github.com/petervdpas/formidable2/internal/log"
)

func TestManagerRecent_NilBroadcaster(t *testing.T) {
	m := NewManager(nil, "", nil)
	got := m.Recent(10)
	if got == nil {
		t.Fatal("Recent should return a non-nil empty slice, not nil")
	}
	if len(got) != 0 {
		t.Errorf("Recent(nil bc) = %d entries, want 0", len(got))
	}
}

func TestManagerRecent_DelegatesToBroadcaster(t *testing.T) {
	bc := applog.NewBroadcaster(8)
	m := NewManager(bc, "", testLogger(bc))
	m.WriteFromFrontend("info", "one", nil)
	m.WriteFromFrontend("info", "two", nil)

	if got := m.Recent(0); len(got) != 2 {
		t.Errorf("Recent(0) = %d, want 2", len(got))
	}
	if got := m.Recent(1); len(got) != 1 || got[0].Msg != "two" {
		t.Errorf("Recent(1) = %v, want [two]", got)
	}
}

func TestManagerReadFile_EmptyPath(t *testing.T) {
	m := NewManager(nil, "", nil)
	body, err := m.ReadFile()
	if err != nil || body != "" {
		t.Errorf("ReadFile(empty path) = (%q, %v), want (\"\", nil)", body, err)
	}
}

func TestManagerReadFile_MissingFileIsNotError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.log")
	m := NewManager(nil, missing, nil)
	body, err := m.ReadFile()
	if err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
	if body != "" {
		t.Errorf("missing file body = %q, want empty", body)
	}
}

func TestManagerReadFile_ReturnsContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "formidable.log")
	if err := os.WriteFile(path, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := NewManager(nil, path, nil)
	body, err := m.ReadFile()
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if body != "line one\nline two\n" {
		t.Errorf("ReadFile = %q", body)
	}
}

func TestManagerReadFile_RealIOErrorBubbles(t *testing.T) {
	// Point logPath at a directory: os.ReadFile fails with a non-IsNotExist
	// error, which must propagate rather than being swallowed as empty.
	dir := t.TempDir()
	m := NewManager(nil, dir, nil)
	_, err := m.ReadFile()
	if err == nil {
		t.Error("reading a directory as a file should return an error")
	}
}

func TestManagerLogPath(t *testing.T) {
	if got := NewManager(nil, "", nil).LogPath(); got != "" {
		t.Errorf("LogPath(off) = %q, want empty", got)
	}
	if got := NewManager(nil, "/var/log/x.log", nil).LogPath(); got != "/var/log/x.log" {
		t.Errorf("LogPath = %q", got)
	}
}

func TestService_Delegates(t *testing.T) {
	bc := applog.NewBroadcaster(8)
	path := filepath.Join(t.TempDir(), "formidable.log")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := NewService(NewManager(bc, path, testLogger(bc)))

	if svc.LogPath() != path {
		t.Errorf("Service.LogPath = %q, want %q", svc.LogPath(), path)
	}
	if body, _ := svc.ReadFile(); body != "hi" {
		t.Errorf("Service.ReadFile = %q, want hi", body)
	}
	if got := svc.Recent(0); len(got) != 0 {
		t.Errorf("Service.Recent on fresh ring = %d, want 0", len(got))
	}
}

func TestServiceLogFromFrontend_AlwaysNilAndWrites(t *testing.T) {
	bc := applog.NewBroadcaster(8)
	svc := NewService(NewManager(bc, "", testLogger(bc)))

	if err := svc.LogFromFrontend("error", "boom", map[string]any{"code": 7}); err != nil {
		t.Errorf("LogFromFrontend should never return an error, got %v", err)
	}
	got := bc.Recent(0)
	if len(got) != 1 || got[0].Level != "ERROR" || got[0].Msg != "boom" {
		t.Fatalf("entry = %+v, want ERROR/boom", got)
	}
	if got[0].Attrs["code"] != int64(7) && got[0].Attrs["code"] != 7 {
		t.Errorf("attr code = %v (%T), want 7", got[0].Attrs["code"], got[0].Attrs["code"])
	}
}

func TestServiceLogFromFrontend_BadInputStillNil(t *testing.T) {
	bc := applog.NewBroadcaster(8)
	svc := NewService(NewManager(bc, "", testLogger(bc)))

	// Empty message and unknown level: dropped / coerced, never an error.
	if err := svc.LogFromFrontend("", "", nil); err != nil {
		t.Errorf("empty input returned error: %v", err)
	}
	if err := svc.LogFromFrontend("bogus-level", "kept", nil); err != nil {
		t.Errorf("unknown level returned error: %v", err)
	}
	got := bc.Recent(0)
	if len(got) != 1 || got[0].Msg != "kept" || got[0].Level != "INFO" {
		t.Errorf("entries = %v, want one INFO 'kept'", got)
	}
}
