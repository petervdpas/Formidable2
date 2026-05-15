package pdf

import (
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

// newTestManagerWithDirs builds a Manager whose prober + store are
// in-memory and whose dirOK accepts the supplied directory whitelist.
func newTestManagerWithDirs(t *testing.T, dirs ...string) (*Manager, *memFS) {
	t.Helper()
	m, mem, _, _ := newTestManager(t)
	allow := map[string]bool{}
	for _, d := range dirs {
		allow[d] = true
	}
	m.dirOK = func(p string) bool { return allow[p] }
	return m, mem
}

func TestSetExportDir_AcceptsValidAbsoluteDir(t *testing.T) {
	m, mem := newTestManagerWithDirs(t, "/exports")
	st, err := m.SetExportDir("/exports")
	if err != nil {
		t.Fatalf("SetExportDir err = %v", err)
	}
	if st.ExportDir != "/exports" {
		t.Errorf("status.ExportDir = %q, want /exports", st.ExportDir)
	}
	if m.Status().ExportDir != "/exports" {
		t.Errorf("Manager.Status().ExportDir = %q, want /exports", m.Status().ExportDir)
	}
	loaded, _ := m.store.Load()
	if loaded.ExportDir != "/exports" {
		t.Errorf("persisted state.ExportDir = %q, want /exports", loaded.ExportDir)
	}
	if !mem.FileExists(stateFilePath) {
		t.Errorf("SetExportDir did not persist state file")
	}
}

func TestSetExportDir_EmptyClearsValue(t *testing.T) {
	m, _ := newTestManagerWithDirs(t, "/exports")
	if _, err := m.SetExportDir("/exports"); err != nil {
		t.Fatalf("seed SetExportDir: %v", err)
	}
	st, err := m.SetExportDir("")
	if err != nil {
		t.Fatalf("SetExportDir(\"\") err = %v", err)
	}
	if st.ExportDir != "" {
		t.Errorf("status.ExportDir = %q, want empty", st.ExportDir)
	}
	loaded, _ := m.store.Load()
	if loaded.ExportDir != "" {
		t.Errorf("persisted state.ExportDir = %q, want empty", loaded.ExportDir)
	}
}

func TestSetExportDir_RelativePathRejected(t *testing.T) {
	m, _ := newTestManagerWithDirs(t)
	_, err := m.SetExportDir("relative/path")
	if !errors.Is(err, ErrInvalidExportDir) {
		t.Errorf("err = %v, want ErrInvalidExportDir", err)
	}
}

func TestSetExportDir_MissingPathRejected(t *testing.T) {
	m, _ := newTestManagerWithDirs(t) // no allowed dirs
	_, err := m.SetExportDir("/no/such/dir")
	if !errors.Is(err, ErrInvalidExportDir) {
		t.Errorf("err = %v, want ErrInvalidExportDir", err)
	}
	if m.Status().ExportDir != "" {
		t.Errorf("failed SetExportDir mutated status: %+v", m.Status())
	}
}

func TestSetExportDir_PreservesActivation(t *testing.T) {
	m, _, fs, vers := newTestManager(t)
	m.dirOK = func(p string) bool { return p == "/exports" }
	fs["/usr/bin/chromium"] = true
	vers["/usr/bin/chromium"] = struct {
		version string
		err     error
	}{version: "Chromium 148.0", err: nil}
	if _, err := m.Activate(ActivateOpts{BrowserBin: "/usr/bin/chromium"}); err != nil {
		t.Fatalf("seed Activate: %v", err)
	}

	st, err := m.SetExportDir("/exports")
	if err != nil {
		t.Fatalf("SetExportDir err = %v", err)
	}
	if !st.Active || st.BrowserBin != "/usr/bin/chromium" {
		t.Errorf("activation state changed after SetExportDir: %+v", st)
	}
	if st.ExportDir != "/exports" {
		t.Errorf("ExportDir = %q, want /exports", st.ExportDir)
	}
}

func TestDeactivate_PreservesExportDir(t *testing.T) {
	m, _, fs, vers := newTestManager(t)
	m.dirOK = func(p string) bool { return p == "/exports" }
	fs["/usr/bin/chromium"] = true
	vers["/usr/bin/chromium"] = struct {
		version string
		err     error
	}{version: "v1", err: nil}
	if _, err := m.Activate(ActivateOpts{BrowserBin: "/usr/bin/chromium"}); err != nil {
		t.Fatalf("seed Activate: %v", err)
	}
	if _, err := m.SetExportDir("/exports"); err != nil {
		t.Fatalf("seed SetExportDir: %v", err)
	}

	if err := m.Deactivate(); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	st := m.Status()
	if st.Active {
		t.Errorf("Active after Deactivate")
	}
	if st.ExportDir != "/exports" {
		t.Errorf("ExportDir wiped by Deactivate; got %q, want /exports", st.ExportDir)
	}
	loaded, _ := m.store.Load()
	if loaded.ExportDir != "/exports" {
		t.Errorf("persisted ExportDir wiped; got %q, want /exports", loaded.ExportDir)
	}
}

func TestRestore_LoadsExportDir(t *testing.T) {
	m, _, fs, _ := newTestManager(t)
	m.dirOK = func(p string) bool { return p == "/exports" }
	fs["/usr/bin/chromium"] = true
	_ = m.store.Save(state{
		BrowserBin:  "/usr/bin/chromium",
		Source:      SourceSystem,
		Version:     "v1",
		ActivatedAt: time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC),
		ExportDir:   "/exports",
	})

	if err := m.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if m.Status().ExportDir != "/exports" {
		t.Errorf("Restore did not load ExportDir; got %q", m.Status().ExportDir)
	}
}

func TestRestore_DropsStaleExportDir(t *testing.T) {
	// Activation path is valid (file present) but ExportDir is gone.
	m, _, fs, _ := newTestManager(t)
	m.dirOK = func(p string) bool { return false } // no dirs exist
	fs["/usr/bin/chromium"] = true
	_ = m.store.Save(state{
		BrowserBin:  "/usr/bin/chromium",
		Source:      SourceSystem,
		ActivatedAt: time.Now(),
		ExportDir:   "/gone/forever",
	})

	if err := m.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !m.Status().Active {
		t.Errorf("stale ExportDir should not affect activation")
	}
	if m.Status().ExportDir != "" {
		t.Errorf("stale ExportDir not cleared; got %q", m.Status().ExportDir)
	}
	loaded, _ := m.store.Load()
	if loaded.ExportDir != "" {
		t.Errorf("stale ExportDir not cleared on disk; got %q", loaded.ExportDir)
	}
}

func TestStore_RoundTripExportDir(t *testing.T) {
	memfs := newMemFS()
	s := &store{fs: memfs, log: slog.Default()}
	if err := s.Save(state{
		BrowserBin: "/usr/bin/chromium",
		Source:     SourceSystem,
		ExportDir:  "/exports",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ExportDir != "/exports" {
		t.Errorf("ExportDir round-trip mismatch; got %q", got.ExportDir)
	}
}

func TestSetExportDir_PathCleaned(t *testing.T) {
	// A user-pasted path with trailing slash + redundant separators
	// should be cleaned via filepath.Clean before validation +
	// persistence. Validation runs against the cleaned form.
	clean := filepath.Clean("/exports/sub/")
	m, _ := newTestManagerWithDirs(t, clean)
	st, err := m.SetExportDir("/exports/sub/")
	if err != nil {
		t.Fatalf("SetExportDir err = %v", err)
	}
	if st.ExportDir != clean {
		t.Errorf("ExportDir not cleaned; got %q, want %q", st.ExportDir, clean)
	}
}
