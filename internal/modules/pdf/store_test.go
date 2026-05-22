package pdf

import (
	"errors"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// memFS is an in-memory storeFS so tests don't touch real disk.
// Missing-file reads return fs.ErrNotExist so the store's
// isMissingErr branch is exercised exactly as in production.
//
// The internal map is guarded by mu so parallel-Export tests don't
// trip the race detector. Production storeFS is *system.Manager,
// whose SaveFile is already serialized at the os.Rename level.
type memFS struct {
	mu        sync.Mutex
	files     map[string]string
	saveErr   error
	loadErr   error
	deleteErr error
}

func newMemFS() *memFS { return &memFS{files: map[string]string{}} }

func (m *memFS) FileExists(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[path]
	return ok
}

func (m *memFS) LoadFile(path string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.files[path] = content
	return nil
}

// DeleteFile mirrors system.Manager.DeleteFile: missing key is a no-op
// (matches journal-aware production semantics). Honors a per-fixture
// deleteErr if a test wants to simulate disk failure.
func (m *memFS) DeleteFile(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.files, path)
	return nil
}

// ResolvePath mimics system.Manager.ResolvePath but without an
// AppRoot - tests already write absolute-ish keys into the map, so
// returning the joined path verbatim keeps the contract simple.
func (m *memFS) ResolvePath(segments ...string) string {
	return strings.Join(segments, "/")
}

// ListDir mimics system.Manager.ListDir: returns the names of files
// directly under `path` (no recursion). Missing path → empty slice.
func (m *memFS) ListDir(path string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := path
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	seen := map[string]bool{}
	out := []string{}
	for k := range m.files {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		// Only direct children - split on / and take the first segment.
		if i := strings.Index(rest, "/"); i >= 0 {
			rest = rest[:i]
		}
		if rest == "" || seen[rest] {
			continue
		}
		seen[rest] = true
		out = append(out, rest)
	}
	return out, nil
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
