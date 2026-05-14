package pdf

import (
	"errors"
	"io/fs"
	"log/slog"
	"testing"
	"time"
)

// memFS is an in-memory storeFS so tests don't touch real disk.
// Missing-file reads return fs.ErrNotExist so the store's
// isMissingErr branch is exercised exactly as in production.
type memFS struct {
	files   map[string]string
	saveErr error
	loadErr error
}

func newMemFS() *memFS { return &memFS{files: map[string]string{}} }

func (m *memFS) FileExists(path string) bool { _, ok := m.files[path]; return ok }

func (m *memFS) LoadFile(path string) (string, error) {
	if m.loadErr != nil {
		return "", m.loadErr
	}
	v, ok := m.files[path]
	if !ok {
		return "", fs.ErrNotExist
	}
	return v, nil
}

func (m *memFS) SaveFile(path, content string) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.files[path] = content
	return nil
}

func TestStore_LoadMissingReturnsUnsetState(t *testing.T) {
	s := &store{fs: newMemFS(), log: slog.Default()}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load on empty fs returned err = %v", err)
	}
	if got.Source != SourceUnset {
		t.Errorf("Source = %q, want unset", got.Source)
	}
	if got.BrowserBin != "" || got.Version != "" {
		t.Errorf("got non-zero state on missing file: %+v", got)
	}
}

func TestStore_SaveLoadRoundTrip(t *testing.T) {
	fs := newMemFS()
	s := &store{fs: fs, log: slog.Default()}
	in := state{
		BrowserBin:  "/usr/bin/chromium",
		Source:      SourceSystem,
		Version:     "Chromium 148.0",
		ActivatedAt: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
	}
	if err := s.Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.BrowserBin != in.BrowserBin || got.Source != in.Source ||
		got.Version != in.Version || !got.ActivatedAt.Equal(in.ActivatedAt) {
		t.Errorf("round-trip mismatch:\ngot  %+v\nwant %+v", got, in)
	}
	if !fs.FileExists(stateFilePath) {
		t.Errorf("Save did not produce file at %q", stateFilePath)
	}
}

func TestStore_LoadMalformedReturnsUnsetAndKeepsFile(t *testing.T) {
	fs := newMemFS()
	fs.files[stateFilePath] = "not json at all"
	s := &store{fs: fs, log: slog.Default()}
	got, err := s.Load()
	if err != nil {
		t.Errorf("Load on malformed json should be tolerant; got err = %v", err)
	}
	if got.Source != SourceUnset {
		t.Errorf("Source on malformed = %q, want unset", got.Source)
	}
	if !fs.FileExists(stateFilePath) {
		t.Errorf("malformed file should not be deleted by Load")
	}
}

func TestStore_ClearRemovesActivation(t *testing.T) {
	fs := newMemFS()
	s := &store{fs: fs, log: slog.Default()}
	_ = s.Save(state{BrowserBin: "/x", Source: SourceSystem, ActivatedAt: time.Now()})
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	got, _ := s.Load()
	if got.Source != SourceUnset {
		t.Errorf("after Clear, Source = %q, want unset", got.Source)
	}
	if got.BrowserBin != "" {
		t.Errorf("after Clear, BrowserBin = %q, want empty", got.BrowserBin)
	}
}

func TestStore_SaveErrorPropagated(t *testing.T) {
	fs := newMemFS()
	fs.saveErr = errors.New("disk full")
	s := &store{fs: fs, log: slog.Default()}
	if err := s.Save(state{Source: SourceSystem}); err == nil {
		t.Errorf("Save with disk error returned nil; want error")
	}
}

func TestStore_LoadErrorOtherThanMissingPropagated(t *testing.T) {
	fs := newMemFS()
	fs.files[stateFilePath] = `{"source":"system"}`
	fs.loadErr = errors.New("permission denied")
	s := &store{fs: fs, log: slog.Default()}
	_, err := s.Load()
	if err == nil {
		t.Errorf("Load with permission error returned nil; want error")
	}
}

func TestStore_NilFSReturnsUnsetState(t *testing.T) {
	s := &store{fs: nil, log: slog.Default()}
	got, err := s.Load()
	if err != nil {
		t.Errorf("nil fs Load err = %v, want nil", err)
	}
	if got.Source != SourceUnset {
		t.Errorf("nil fs Source = %q, want unset", got.Source)
	}
	if err := s.Save(state{Source: SourceSystem}); err != nil {
		t.Errorf("nil fs Save err = %v, want nil (no-op)", err)
	}
}
