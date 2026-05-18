package pdf

import (
	"log/slog"
	"strings"
	"testing"
)

// Setup: scaffolded memFS so the central images/ dir is populated
// with formidable.svg (the default seed).
func newCoverLogoFS(t *testing.T) *memFS {
	t.Helper()
	fs := newMemFS()
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	return fs
}

func TestResolveCoverLogo_EmptyStaysEmpty(t *testing.T) {
	fs := newCoverLogoFS(t)
	got := ResolveCoverLogo("", "/storage/tpl", fs)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveCoverLogo_AbsolutePathPassthrough(t *testing.T) {
	fs := newCoverLogoFS(t)
	in := "/abs/path/team-logo.png"
	got := ResolveCoverLogo(in, "/storage/tpl", fs)
	if got != in {
		t.Errorf("got %q, want %q (absolute passthrough)", got, in)
	}
}

func TestResolveCoverLogo_BareFilename_ResolvesToImagesDir(t *testing.T) {
	fs := newCoverLogoFS(t)
	got := ResolveCoverLogo("formidable.svg", "/storage/tpl", fs)
	want := "pdf/covers/images/formidable.svg"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveCoverLogo_BareFilename_FallsBackToSourceDir(t *testing.T) {
	fs := newCoverLogoFS(t)
	// Drop a logo into the form's storage dir (NOT in the central
	// images dir). Bare-filename lookup should still find it.
	fs.files["/storage/tpl/team-photo.png"] = "fake-png-bytes"
	got := ResolveCoverLogo("team-photo.png", "/storage/tpl", fs)
	if got != "/storage/tpl/team-photo.png" {
		t.Errorf("got %q, want sourceDir-relative resolution", got)
	}
}

func TestResolveCoverLogo_BareFilename_CentralWinsOverSourceDir(t *testing.T) {
	fs := newCoverLogoFS(t)
	// Same name in both places — central wins so users get the
	// gigot-synced version, not whatever happens to live in storage.
	fs.files["/storage/tpl/formidable.svg"] = "wrong-one"
	got := ResolveCoverLogo("formidable.svg", "/storage/tpl", fs)
	want := "pdf/covers/images/formidable.svg"
	if got != want {
		t.Errorf("got %q, want %q (central library should win)", got, want)
	}
}

func TestResolveCoverLogo_RelativeWithSlashes_ResolvesAgainstSourceDir(t *testing.T) {
	fs := newCoverLogoFS(t)
	fs.files["/storage/tpl/assets/team.png"] = "x"
	got := ResolveCoverLogo("assets/team.png", "/storage/tpl", fs)
	if got != "/storage/tpl/assets/team.png" {
		t.Errorf("got %q, want sourceDir-relative resolution", got)
	}
}

func TestResolveCoverLogo_RelativeWithSlashes_FallsBackToImagesByBasename(t *testing.T) {
	fs := newCoverLogoFS(t)
	// User's path doesn't exist relative to sourceDir; basename does
	// exist in the central images dir. Resolver finds it.
	got := ResolveCoverLogo("./team/formidable.svg", "/storage/tpl", fs)
	want := "pdf/covers/images/formidable.svg"
	if got != want {
		t.Errorf("got %q, want %q (basename fallback to central dir)", got, want)
	}
}

func TestResolveCoverLogo_NotFoundReturnsInputUnchanged(t *testing.T) {
	fs := newCoverLogoFS(t)
	in := "nope-not-real.png"
	got := ResolveCoverLogo(in, "/storage/tpl", fs)
	if got != in {
		t.Errorf("got %q, want %q (unresolved returns input)", got, in)
	}
}

func TestResolveCoverLogo_NilFSReturnsInput(t *testing.T) {
	got := ResolveCoverLogo("formidable.svg", "/storage/tpl", nil)
	if got != "formidable.svg" {
		t.Errorf("got %q, want input unchanged when fs nil", got)
	}
}

// winFS wraps memFS but emits backslashed absolute paths from
// ResolvePath, simulating Windows' filepath.Abs return value. The
// only purpose is to verify ResolveCoverLogo normalises the slash
// direction on the way out — Chrome cannot resolve an `<img src>`
// containing literal backslashes when rendering a file:// document.
type winFS struct{ *memFS }

func (w *winFS) ResolvePath(segments ...string) string {
	joined := w.memFS.ResolvePath(segments...)
	return "C:" + strings.ReplaceAll(joined, "/", `\`)
}

func TestResolveCoverLogo_NormalisesBackslashesToForwardSlashes(t *testing.T) {
	fs := &winFS{memFS: newMemFS()}
	if err := scaffoldCovers(fs.memFS, slog.Default()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	got := ResolveCoverLogo("formidable.svg", "/storage/tpl", fs)
	if strings.Contains(got, `\`) {
		t.Errorf("got %q, must not contain backslashes (Chrome chokes on Windows-native paths in <img src>)", got)
	}
	if !strings.Contains(got, "pdf/covers/images/formidable.svg") {
		t.Errorf("got %q, want forward-slashed pdf/covers/images/formidable.svg substring", got)
	}
}
