package system

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestJoinPath(t *testing.T) {
	m, root := newTestManager(t)

	if got := m.JoinPath(); got != root {
		t.Errorf("JoinPath() = %q, want AppRoot %q", got, root)
	}

	wantRel := filepath.Join(root, "a", "b")
	if got := m.JoinPath("a", "b"); got != wantRel {
		t.Errorf("JoinPath(rel) = %q, want %q", got, wantRel)
	}

	abs := filepath.Join(t.TempDir(), "elsewhere")
	if got := m.JoinPath(abs, "leaf"); got != filepath.Join(abs, "leaf") {
		t.Errorf("JoinPath(abs first) = %q, want %q", got, filepath.Join(abs, "leaf"))
	}
}

func TestSetAppRoot_AffectsJoinPath(t *testing.T) {
	m, _ := newTestManager(t)
	newRoot := t.TempDir()
	m.SetAppRoot(newRoot)
	if m.AppRoot() != newRoot {
		t.Errorf("AppRoot after SetAppRoot = %q, want %q", m.AppRoot(), newRoot)
	}
	if got := m.JoinPath("x"); got != filepath.Join(newRoot, "x") {
		t.Errorf("JoinPath after SetAppRoot = %q, want under %q", got, newRoot)
	}
}

func TestIsDir_True(t *testing.T) {
	m, root := newTestManager(t)
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if !m.IsDir("sub") {
		t.Error("IsDir(existing dir) = false, want true")
	}
}

func TestIsDir_FileIsNotDir(t *testing.T) {
	m, root := newTestManager(t)
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if m.IsDir("f.txt") {
		t.Error("IsDir(regular file) = true, want false")
	}
}

func TestIsDir_MissingPath(t *testing.T) {
	m, _ := newTestManager(t)
	if m.IsDir("nope") {
		t.Error("IsDir(missing) = true, want false")
	}
}

func TestIsDir_AbsolutePath(t *testing.T) {
	m, _ := newTestManager(t)
	other := t.TempDir() // absolute, outside AppRoot
	if !m.IsDir(other) {
		t.Error("IsDir(absolute existing dir) = false, want true")
	}
}

func TestListDir_ReturnsEntryNames(t *testing.T) {
	m, root := newTestManager(t)
	d := filepath.Join(root, "data")
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := os.Mkdir(filepath.Join(d, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, err := m.ListDir("data")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	sort.Strings(got)
	want := []string{"a.txt", "b.txt", "nested"}
	if len(got) != len(want) {
		t.Fatalf("ListDir = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListDir[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestListDir_EmptyDirReturnsEmptyNotNilError(t *testing.T) {
	m, root := newTestManager(t)
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := m.ListDir("empty")
	if err != nil {
		t.Fatalf("ListDir(empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListDir(empty) = %v, want no entries", got)
	}
}

func TestListDir_MissingDirIsNotError(t *testing.T) {
	m, _ := newTestManager(t)
	got, err := m.ListDir("does-not-exist")
	if err != nil {
		t.Errorf("ListDir(missing) should not error, got %v", err)
	}
	if got != nil {
		t.Errorf("ListDir(missing) = %v, want nil", got)
	}
}

func TestListDir_PathIsFileBubblesError(t *testing.T) {
	m, root := newTestManager(t)
	if err := os.WriteFile(filepath.Join(root, "afile"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// ReadDir on a regular file is a real I/O error (not IsNotExist) and
	// must propagate rather than be swallowed as "no files yet".
	if _, err := m.ListDir("afile"); err == nil {
		t.Error("ListDir(regular file) = nil error, want error")
	}
}
