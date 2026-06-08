package sfr

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

func newTestManager(t *testing.T) (*Manager, *system.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	return NewManager(sys, nil), sys, root
}

func boolPtr(b bool) *bool { return &b }

// ----- storagePath validation -------------------------------------------

func TestStoragePath_Defaults(t *testing.T) {
	m, _, root := newTestManager(t)
	got, err := m.storagePath("storage/basic", "form-1", Options{})
	if err != nil {
		t.Fatalf("storagePath: %v", err)
	}
	want := filepath.Join(root, "storage/basic", "form-1.meta.json")
	if got != want {
		t.Errorf("storagePath = %q, want %q", got, want)
	}
}

func TestStoragePath_StripsMd(t *testing.T) {
	m, _, _ := newTestManager(t)
	got, _ := m.storagePath("d", "form.md", Options{})
	if !strings.HasSuffix(got, "form.meta.json") {
		t.Errorf("md not stripped: %s", got)
	}
}

func TestStoragePath_StripsConfiguredExt(t *testing.T) {
	m, _, _ := newTestManager(t)
	got, _ := m.storagePath("d", "form.meta.json", Options{})
	if strings.HasSuffix(got, ".meta.json.meta.json") {
		t.Errorf("ext doubled: %s", got)
	}
	if !strings.HasSuffix(got, "form.meta.json") {
		t.Errorf("expected form.meta.json suffix, got %s", got)
	}
}

func TestStoragePath_CustomExtensionWithoutLeadingDot(t *testing.T) {
	m, _, _ := newTestManager(t)
	got, err := m.storagePath("d", "x", Options{Extension: "json"})
	if err != nil {
		t.Fatalf("storagePath: %v", err)
	}
	if !strings.HasSuffix(got, "x.json") {
		t.Errorf("expected x.json suffix, got %s", got)
	}
}

func TestStoragePath_RejectsTraversal(t *testing.T) {
	m, _, _ := newTestManager(t)
	bad := []string{
		"../escape",
		"sub/file",
		"sub\\file",
		"..",
		".",
		"x..y",
		"",
	}
	for _, b := range bad {
		if _, err := m.storagePath("d", b, Options{}); err == nil {
			t.Errorf("storagePath(%q) should error", b)
		}
	}
}

// ----- Encode / decode ---------------------------------------------------

func TestEncode_DefaultJSON(t *testing.T) {
	m, _, _ := newTestManager(t)
	out, err := m.encode(map[string]any{"a": 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"a"`) {
		t.Errorf("expected JSON output, got %q", out)
	}
}

func TestEncode_TextModeRequiresString(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.encode(123, Options{JSON: boolPtr(false)}); err == nil {
		t.Error("expected error for non-string in text mode")
	}
	if out, err := m.encode("hello", Options{JSON: boolPtr(false)}); err != nil || out != "hello" {
		t.Errorf("text mode: got (%q, %v)", out, err)
	}
	if out, err := m.encode(nil, Options{JSON: boolPtr(false)}); err != nil || out != "" {
		t.Errorf("text mode nil: got (%q, %v)", out, err)
	}
}

func TestDecode_FallsBackToRawOnBadJSON(t *testing.T) {
	m, _, _ := newTestManager(t)
	out := m.decode("not valid json {[}", Options{})
	if s, ok := out.(string); !ok || s != "not valid json {[}" {
		t.Errorf("expected raw fallback, got %v", out)
	}
}

func TestDecode_TextMode(t *testing.T) {
	m, _, _ := newTestManager(t)
	out := m.decode(`{"a":1}`, Options{JSON: boolPtr(false)})
	if s, ok := out.(string); !ok || s != `{"a":1}` {
		t.Errorf("text mode should not parse JSON, got %v", out)
	}
}

// ----- Round-trip via Manager (defense-in-depth, separate from godog) ---

func TestSaveLoad_TextModeRoundTrip(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.SaveFromBase("d", "x", "raw text body", Options{
		Extension: ".txt",
		JSON:      boolPtr(false),
	})
	if !r.Success {
		t.Fatalf("save failed: %+v", r)
	}
	got, err := m.LoadFromBase("d", "x", Options{
		Extension: ".txt",
		JSON:      boolPtr(false),
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != "raw text body" {
		t.Errorf("round-trip mismatch: %v", got)
	}
}

func TestListFiles_DirectoryDoesNotExist(t *testing.T) {
	m, _, _ := newTestManager(t)
	files, err := m.ListFiles("never/created", "")
	if err == nil {
		t.Fatalf("expected error for missing dir, got files=%v", files)
	}
}

func TestDeleteFromBase_MissingIsNotAnError(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.DeleteFromBase("d", "missing", Options{}); err != nil {
		t.Errorf("delete missing should be no-op: %v", err)
	}
}

// fakeFS implements the sfr fs interface with per-method controllable
// errors so each failure branch can be exercised in isolation.
type fakeFS struct {
	exists     bool
	loadOut    string
	loadErr    error
	saveErr    error
	deleteErr  error
	ensureErr  error
	listOut    []string
	listErr    error
	deleteSeen string
}

func (f *fakeFS) ResolvePath(segments ...string) string {
	return filepath.Join(append([]string{"/root"}, segments...)...)
}
func (f *fakeFS) EnsureDirectory(string) error       { return f.ensureErr }
func (f *fakeFS) FileExists(string) bool             { return f.exists }
func (f *fakeFS) LoadFile(string) (string, error)    { return f.loadOut, f.loadErr }
func (f *fakeFS) SaveFile(string, string) error      { return f.saveErr }
func (f *fakeFS) DeleteFile(p string) error          { f.deleteSeen = p; return f.deleteErr }
func (f *fakeFS) ListFiles(string) ([]string, error) { return f.listOut, f.listErr }

func TestStoragePath_RejectsEmptyBase(t *testing.T) {
	m := NewManager(&fakeFS{}, nil)
	r := m.SaveFromBase("dir", "", "x", Options{})
	if r.Success || r.Error == "" {
		t.Errorf("empty base should fail, got %+v", r)
	}
}

func TestStoragePath_RejectsPathSeparators(t *testing.T) {
	m := NewManager(&fakeFS{}, nil)
	for _, base := range []string{"a/b", `a\b`} {
		r := m.SaveFromBase("dir", base, "x", Options{})
		if r.Success || r.Error == "" {
			t.Errorf("base %q with separator should fail, got %+v", base, r)
		}
	}
}

func TestStoragePath_RejectsDotDotTraversal(t *testing.T) {
	m := NewManager(&fakeFS{}, nil)
	for _, base := range []string{"..", ".", "x..y"} {
		r := m.SaveFromBase("dir", base, "x", Options{})
		if r.Success || r.Error == "" {
			t.Errorf("base %q should be rejected as traversal, got %+v", base, r)
		}
	}
}

func TestStoragePath_EnsureDirectoryError(t *testing.T) {
	m := NewManager(&fakeFS{ensureErr: errors.New("mkdir denied")}, nil)
	r := m.SaveFromBase("dir", "ok", "x", Options{})
	if r.Success || r.Error == "" {
		t.Errorf("EnsureDirectory failure should surface, got %+v", r)
	}
}

func TestSaveFromBase_SaveFileError(t *testing.T) {
	m := NewManager(&fakeFS{saveErr: errors.New("disk full")}, nil)
	r := m.SaveFromBase("dir", "ok", map[string]any{"a": 1}, Options{})
	if r.Success || r.Error == "" {
		t.Errorf("SaveFile failure should surface, got %+v", r)
	}
}

func TestSaveFromBase_TextModeNonStringDataErrors(t *testing.T) {
	no := false
	m := NewManager(&fakeFS{}, nil)
	r := m.SaveFromBase("dir", "ok", 42, Options{JSON: &no}) // text mode + int
	if r.Success || r.Error == "" {
		t.Errorf("text mode with non-string data should fail, got %+v", r)
	}
}

func TestLoadFromBase_StoragePathError(t *testing.T) {
	m := NewManager(&fakeFS{}, nil)
	if _, err := m.LoadFromBase("dir", "", Options{}); err == nil {
		t.Error("empty base should error from LoadFromBase")
	}
}

func TestLoadFromBase_FileNotFound(t *testing.T) {
	m := NewManager(&fakeFS{exists: false}, nil)
	_, err := m.LoadFromBase("dir", "ok", Options{})
	if err == nil {
		t.Error("missing file should error")
	}
}

func TestLoadFromBase_LoadFileError(t *testing.T) {
	// File reports as present but the read fails: the LoadFile error path
	// the godog suite never reaches (it only covers file-not-found).
	m := NewManager(&fakeFS{exists: true, loadErr: errors.New("io boom")}, nil)
	_, err := m.LoadFromBase("dir", "ok", Options{})
	if err == nil {
		t.Error("LoadFile failure should propagate")
	}
}

func TestListFiles_ListError(t *testing.T) {
	m := NewManager(&fakeFS{listErr: errors.New("list boom")}, nil)
	if _, err := m.ListFiles("dir", ""); err == nil {
		t.Error("ListFiles failure should propagate")
	}
}

func TestDeleteFromBase_StoragePathError(t *testing.T) {
	m := NewManager(&fakeFS{}, nil)
	if err := m.DeleteFromBase("dir", "", Options{}); err == nil {
		t.Error("empty base should error from DeleteFromBase")
	}
}

func TestDeleteFromBase_DeleteFileError(t *testing.T) {
	m := NewManager(&fakeFS{deleteErr: errors.New("perm denied")}, nil)
	if err := m.DeleteFromBase("dir", "ok", Options{}); err == nil {
		t.Error("DeleteFile failure should propagate")
	}
}
