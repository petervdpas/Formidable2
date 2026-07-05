package fonts

import (
	"errors"
	"path"
	"strings"
	"testing"
	"testing/fstest"
)

// memFS is an in-memory FS for the fonts manager (mirrors the pdf cover-image
// tests). Keys are the resolved relative paths the manager writes.
type memFS struct {
	files   map[string]string
	saveErr error
}

func newMemFS() *memFS { return &memFS{files: map[string]string{}} }

func (m *memFS) FileExists(p string) bool { _, ok := m.files[p]; return ok }
func (m *memFS) LoadFile(p string) (string, error) {
	v, ok := m.files[p]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (m *memFS) SaveFile(p, content string) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.files[p] = content
	return nil
}
func (m *memFS) DeleteFile(p string) error { delete(m.files, p); return nil }
func (m *memFS) ListDir(dir string) ([]string, error) {
	out := []string{}
	prefix := dir + "/"
	for p := range m.files {
		if strings.HasPrefix(p, prefix) && !strings.Contains(strings.TrimPrefix(p, prefix), "/") {
			out = append(out, path.Base(p))
		}
	}
	return out, nil
}

func TestList_EmptyDir(t *testing.T) {
	got, err := NewManager(newMemFS()).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty dir should list 0 fonts, got %d", len(got))
	}
}

func TestList_FiltersSortsAndReportsFamilySize(t *testing.T) {
	fs := newMemFS()
	fs.files["fonts/Zeta.woff2"] = "zzz"
	fs.files["fonts/Alpha.ttf"] = "aa"
	fs.files["fonts/notes.txt"] = "ignore me"
	m := NewManager(fs)
	got, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 fonts (txt filtered), got %d: %+v", len(got), got)
	}
	if got[0].Family != "Alpha" || got[1].Family != "Zeta" {
		t.Errorf("fonts should sort by family, got %q,%q", got[0].Family, got[1].Family)
	}
	if got[0].Filename != "Alpha.ttf" || got[0].Size != 2 {
		t.Errorf("family/size wrong: %+v", got[0])
	}
}

func TestList_FamilyKeepsSpaces(t *testing.T) {
	fs := newMemFS()
	fs.files["fonts/Open Sans.woff2"] = "x"
	got, _ := NewManager(fs).List()
	if len(got) != 1 || got[0].Family != "Open Sans" {
		t.Errorf("family should keep spaces, got %+v", got)
	}
}

func TestSave_WritesToFontsDir(t *testing.T) {
	fs := newMemFS()
	if err := NewManager(fs).Save("Inter.woff2", []byte("bytes")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if fs.files["fonts/Inter.woff2"] != "bytes" {
		t.Errorf("font not written to fonts/Inter.woff2: %+v", fs.files)
	}
}

func TestSave_RejectsTraversalAndBadNames(t *testing.T) {
	m := NewManager(newMemFS())
	for _, name := range []string{"../escape.woff2", "sub/nested.woff2", "..", "", ".woff2"} {
		if err := m.Save(name, []byte("x")); !errors.Is(err, ErrInvalidFont) {
			t.Errorf("Save(%q) should be rejected as invalid, got %v", name, err)
		}
	}
}

func TestSave_RejectsBadExtension(t *testing.T) {
	m := NewManager(newMemFS())
	for _, name := range []string{"evil.txt", "logo.png", "script.exe"} {
		if err := m.Save(name, []byte("x")); !errors.Is(err, ErrInvalidFont) {
			t.Errorf("Save(%q) should reject unsupported extension, got %v", name, err)
		}
	}
}

func TestSave_AcceptsFontExtensionsIncludingUppercase(t *testing.T) {
	m := NewManager(newMemFS())
	for _, name := range []string{"A.woff2", "B.woff", "C.ttf", "D.otf", "E.TTF"} {
		if err := m.Save(name, []byte("x")); err != nil {
			t.Errorf("Save(%q) should be accepted, got %v", name, err)
		}
	}
}

func TestSave_RejectsEmptyBytes(t *testing.T) {
	if err := NewManager(newMemFS()).Save("Inter.woff2", nil); !errors.Is(err, ErrInvalidFont) {
		t.Errorf("empty body should be rejected, got %v", err)
	}
}

func TestLoad_RoundTripsAndMissingErrors(t *testing.T) {
	fs := newMemFS()
	fs.files["fonts/Inter.woff2"] = "the-bytes"
	m := NewManager(fs)
	raw, err := m.Load("Inter.woff2")
	if err != nil || string(raw) != "the-bytes" {
		t.Errorf("Load round-trip failed: %q %v", raw, err)
	}
	if _, err := m.Load("Missing.woff2"); err == nil {
		t.Errorf("loading a missing font should error")
	}
	if _, err := m.Load("../escape.woff2"); !errors.Is(err, ErrInvalidFont) {
		t.Errorf("traversal load should be rejected")
	}
}

func TestDelete_RemovesMissingIsNoopTraversalErrors(t *testing.T) {
	fs := newMemFS()
	fs.files["fonts/Inter.woff2"] = "x"
	m := NewManager(fs)
	if err := m.Delete("Inter.woff2"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fs.FileExists("fonts/Inter.woff2") {
		t.Errorf("font should be gone")
	}
	if err := m.Delete("Inter.woff2"); err != nil {
		t.Errorf("deleting a missing font should be a no-op, got %v", err)
	}
	if err := m.Delete("../escape.woff2"); !errors.Is(err, ErrInvalidFont) {
		t.Errorf("traversal delete should be rejected")
	}
}

func TestScaffold_SeedsFlagRestoreAndNoOverwrite(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs)
	// Inject a fake factory (one font + a README that must be ignored).
	m.seedFS = fstest.MapFS{
		"factory/Brand.woff2": {Data: []byte("BRANDBYTES")},
		"factory/README.md":   {Data: []byte("ignore me")},
	}

	if err := m.Scaffold(); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if fs.files["fonts/Brand.woff2"] != "BRANDBYTES" {
		t.Fatalf("seed not scaffolded to disk: %+v", fs.files)
	}

	got, _ := m.List()
	if len(got) != 1 || !got[0].IsSeed || got[0].Family != "Brand" {
		t.Fatalf("expected one SEED font Brand, got %+v", got)
	}

	// Delete a seed, then Restore (re-Scaffold) brings it back.
	if err := m.Delete("Brand.woff2"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fs.FileExists("fonts/Brand.woff2") {
		t.Fatalf("seed should be deleted before restore")
	}
	if err := m.Scaffold(); err != nil {
		t.Fatalf("restore Scaffold: %v", err)
	}
	if fs.files["fonts/Brand.woff2"] != "BRANDBYTES" {
		t.Fatalf("Restore default fonts must rewrite the deleted seed")
	}

	// Scaffold must never clobber an existing (possibly user-replaced) file.
	fs.files["fonts/Brand.woff2"] = "EDITED"
	_ = m.Scaffold()
	if fs.files["fonts/Brand.woff2"] != "EDITED" {
		t.Fatalf("Scaffold must not overwrite an existing font")
	}
}

func TestFontFaceCSS_EmbedsEachFontAsDataURI(t *testing.T) {
	fs := newMemFS()
	fs.files["fonts/Inter.woff2"] = "AB"
	fs.files["fonts/Serif One.ttf"] = "CD"
	css, err := NewManager(fs).FontFaceCSS()
	if err != nil {
		t.Fatalf("FontFaceCSS: %v", err)
	}
	for _, want := range []string{
		`@font-face{font-family:"Inter";`,
		`@font-face{font-family:"Serif One";`,
		"src:url(data:font/woff2;base64,",
		`format("woff2")`,
		"data:font/ttf;base64,",
		`format("truetype")`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("FontFaceCSS missing %q\n%s", want, css)
		}
	}
}
