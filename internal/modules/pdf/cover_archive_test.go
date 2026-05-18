package pdf

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// validBundlableCover is a minimal cover whose HTML passes ValidateCover
// AND references two images so the scanner has something to bundle.
const validBundlableCover = `<!--
  formidable-cover: 1
  name: Team
  description: Test fixture.
-->
<section class="cover" data-cover-start>
  <h1>{{.Title}}</h1>
  <img src="formidable.svg" alt="Logo">
  <style>.banner{background:url('banner.png')}</style>
</section>
<span data-cover-end></span>`

func newArchiveFS(t *testing.T) *memFS {
	t.Helper()
	fs := newMemFS()
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	return fs
}

// ─── Export ──────────────────────────────────────────────────────────

func TestExportCoverArchive_HappyPath(t *testing.T) {
	fs := newArchiveFS(t)
	fs.files[onDiskCoversDir+"/team.html"] = validBundlableCover
	fs.files[onDiskCoversDir+"/images/banner.png"] = "fake-png-bytes"
	// formidable.svg already seeded by scaffold

	res, err := exportCoverArchive(fs, "team", "/tmp/team.zip")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Name != "team" || res.ZipPath != "/tmp/team.zip" {
		t.Errorf("wrong result name/path: %+v", res)
	}
	if len(res.MissingImages) != 0 {
		t.Errorf("unexpected missing: %v", res.MissingImages)
	}
	wantImages := map[string]bool{"formidable.svg": true, "banner.png": true}
	if len(res.Images) != len(wantImages) {
		t.Fatalf("got %d images, want %d (%v)", len(res.Images), len(wantImages), res.Images)
	}
	for _, n := range res.Images {
		if !wantImages[n] {
			t.Errorf("unexpected image bundled: %q", n)
		}
	}

	raw, err := fs.LoadFile("/tmp/team.zip")
	if err != nil {
		t.Fatalf("zip not written: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader([]byte(raw)), int64(len(raw)))
	if err != nil {
		t.Fatalf("written zip unreadable: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"team.html", "images/formidable.svg", "images/banner.png"} {
		if !names[want] {
			t.Errorf("zip missing entry %q (got %v)", want, names)
		}
	}
}

func TestExportCoverArchive_NoImagesProducesHtmlOnlyZip(t *testing.T) {
	fs := newArchiveFS(t)
	plain := `<!--
  formidable-cover: 1
  name: Plain
-->
<section class="cover" data-cover-start><h1>x</h1></section><span data-cover-end></span>`
	fs.files[onDiskCoversDir+"/plain.html"] = plain

	res, err := exportCoverArchive(fs, "plain", "/tmp/plain.zip")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Images) != 0 {
		t.Errorf("expected no images, got %v", res.Images)
	}
}

func TestExportCoverArchive_RecordsMissingImages(t *testing.T) {
	fs := newArchiveFS(t)
	html := `<!--
  formidable-cover: 1
  name: Half
-->
<section class="cover" data-cover-start>
		<img src="ghost.png">
		<img src="formidable.svg">
		</section><span data-cover-end></span>`
	fs.files[onDiskCoversDir+"/half.html"] = html
	// ghost.png does NOT exist on disk; formidable.svg does (scaffolded).

	res, err := exportCoverArchive(fs, "half", "/tmp/half.zip")
	if err != nil {
		t.Fatalf("export should succeed even with missing image refs: %v", err)
	}
	if len(res.MissingImages) != 1 || res.MissingImages[0] != "ghost.png" {
		t.Errorf("missing images = %v, want [ghost.png]", res.MissingImages)
	}
	if len(res.Images) != 1 || res.Images[0] != "formidable.svg" {
		t.Errorf("bundled images = %v, want [formidable.svg]", res.Images)
	}
}

func TestExportCoverArchive_UnhappyPaths(t *testing.T) {
	fs := newArchiveFS(t)
	fs.files[onDiskCoversDir+"/ok.html"] = validBundlableCover
	fs.files[onDiskCoversDir+"/images/banner.png"] = "x"

	cases := []struct {
		name        string
		coverName   string
		zipPath     string
		wantErrText string
	}{
		{"empty cover name", "", "/tmp/x.zip", "cover name"},
		{"empty zip path", "ok", "", "zip path"},
		{"name with path separator", "sub/ok", "/tmp/x.zip", "invalid"},
		{"name with backslash", `sub\ok`, "/tmp/x.zip", "invalid"},
		{"name with leading dot", ".hidden", "/tmp/x.zip", "invalid"},
		{"reserved signature name", "signature", "/tmp/x.zip", "invalid"},
		{"cover not found on disk", "nope-not-real", "/tmp/x.zip", "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := exportCoverArchive(fs, tc.coverName, tc.zipPath)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErrText)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.wantErrText) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantErrText)
			}
		})
	}
}

func TestExportCoverArchive_NilFS(t *testing.T) {
	_, err := exportCoverArchive(nil, "team", "/tmp/team.zip")
	if err == nil {
		t.Fatalf("expected error with nil fs, got nil")
	}
}

// ─── Import ──────────────────────────────────────────────────────────

// makeZip builds an in-memory zip from the given entries. Entry keys
// are zip-internal paths (slash-separated); values are file bodies.
func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestImportCoverArchive_HappyPath(t *testing.T) {
	fs := newArchiveFS(t)
	zipBytes := makeZip(t, map[string]string{
		"team.html":             validBundlableCover,
		"images/formidable.svg": "<svg>logo</svg>",
		"images/banner.png":     "png-bytes",
	})
	fs.files["/tmp/team.zip"] = string(zipBytes)

	res, err := importCoverArchive(fs, "/tmp/team.zip", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Name != "team" {
		t.Errorf("got name %q, want team", res.Name)
	}
	if res.Overwritten {
		t.Errorf("overwritten should be false on first import")
	}
	if got := fs.files[onDiskCoversDir+"/team.html"]; got != validBundlableCover {
		t.Errorf("html not written or wrong content")
	}
	if got := fs.files[onDiskCoversDir+"/images/banner.png"]; got != "png-bytes" {
		t.Errorf("banner.png not written")
	}
}

func TestImportCoverArchive_OverwriteRefusalByDefault(t *testing.T) {
	fs := newArchiveFS(t)
	fs.files[onDiskCoversDir+"/team.html"] = "existing"
	zipBytes := makeZip(t, map[string]string{"team.html": validBundlableCover})
	fs.files["/tmp/team.zip"] = string(zipBytes)

	_, err := importCoverArchive(fs, "/tmp/team.zip", false)
	if !errors.Is(err, ErrCoverArchiveExists) {
		t.Fatalf("got err %v, want ErrCoverArchiveExists", err)
	}
	if got := fs.files[onDiskCoversDir+"/team.html"]; got != "existing" {
		t.Errorf("existing cover was clobbered: %q", got)
	}
}

func TestImportCoverArchive_OverwriteAllowedWhenFlagSet(t *testing.T) {
	fs := newArchiveFS(t)
	fs.files[onDiskCoversDir+"/team.html"] = "existing"
	zipBytes := makeZip(t, map[string]string{"team.html": validBundlableCover})
	fs.files["/tmp/team.zip"] = string(zipBytes)

	res, err := importCoverArchive(fs, "/tmp/team.zip", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Overwritten {
		t.Errorf("overwritten should be true")
	}
	if got := fs.files[onDiskCoversDir+"/team.html"]; got != validBundlableCover {
		t.Errorf("cover not overwritten")
	}
}

func TestImportCoverArchive_UnhappyPaths(t *testing.T) {
	t.Run("empty zip path", func(t *testing.T) {
		fs := newArchiveFS(t)
		_, err := importCoverArchive(fs, "", false)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})

	t.Run("zip file does not exist", func(t *testing.T) {
		fs := newArchiveFS(t)
		_, err := importCoverArchive(fs, "/tmp/missing.zip", false)
		if !errors.Is(err, ErrCoverArchiveNotFound) {
			t.Fatalf("got err %v, want ErrCoverArchiveNotFound", err)
		}
	})

	t.Run("malformed zip", func(t *testing.T) {
		fs := newArchiveFS(t)
		fs.files["/tmp/bad.zip"] = "not actually a zip"
		_, err := importCoverArchive(fs, "/tmp/bad.zip", false)
		if !errors.Is(err, ErrCoverArchiveInvalid) {
			t.Fatalf("got err %v, want ErrCoverArchiveInvalid", err)
		}
	})

	t.Run("empty zip", func(t *testing.T) {
		fs := newArchiveFS(t)
		fs.files["/tmp/empty.zip"] = string(makeZip(t, map[string]string{}))
		_, err := importCoverArchive(fs, "/tmp/empty.zip", false)
		if !errors.Is(err, ErrCoverArchiveInvalid) {
			t.Fatalf("got err %v, want ErrCoverArchiveInvalid", err)
		}
	})

	t.Run("zip with no html at root", func(t *testing.T) {
		fs := newArchiveFS(t)
		fs.files["/tmp/no-html.zip"] = string(makeZip(t, map[string]string{
			"images/logo.png": "x",
		}))
		_, err := importCoverArchive(fs, "/tmp/no-html.zip", false)
		if !errors.Is(err, ErrCoverArchiveInvalid) {
			t.Fatalf("got err %v, want ErrCoverArchiveInvalid", err)
		}
	})

	t.Run("zip with multiple html at root", func(t *testing.T) {
		fs := newArchiveFS(t)
		fs.files["/tmp/multi.zip"] = string(makeZip(t, map[string]string{
			"a.html": validBundlableCover,
			"b.html": validBundlableCover,
		}))
		_, err := importCoverArchive(fs, "/tmp/multi.zip", false)
		if !errors.Is(err, ErrCoverArchiveInvalid) {
			t.Fatalf("got err %v, want ErrCoverArchiveInvalid", err)
		}
	})

	t.Run("zip html name fails cover-stem validation", func(t *testing.T) {
		fs := newArchiveFS(t)
		fs.files["/tmp/bad-name.zip"] = string(makeZip(t, map[string]string{
			"signature.html": validBundlableCover,
		}))
		_, err := importCoverArchive(fs, "/tmp/bad-name.zip", false)
		if !errors.Is(err, ErrCoverArchiveInvalid) {
			t.Fatalf("got err %v, want ErrCoverArchiveInvalid", err)
		}
	})

	t.Run("zip with path traversal", func(t *testing.T) {
		fs := newArchiveFS(t)
		fs.files["/tmp/trav.zip"] = string(makeZip(t, map[string]string{
			"team.html":              validBundlableCover,
			"../../etc/passwd":       "pwned",
			"images/../../../oops":   "no",
		}))
		_, err := importCoverArchive(fs, "/tmp/trav.zip", false)
		if !errors.Is(err, ErrCoverArchiveTraversal) {
			t.Fatalf("got err %v, want ErrCoverArchiveTraversal", err)
		}
		// Refused → nothing materialised:
		if _, exists := fs.files[onDiskCoversDir+"/team.html"]; exists {
			t.Errorf("cover html was written despite traversal in same zip")
		}
	})

	t.Run("zip entry outside expected layout", func(t *testing.T) {
		fs := newArchiveFS(t)
		fs.files["/tmp/extra.zip"] = string(makeZip(t, map[string]string{
			"team.html":     validBundlableCover,
			"unexpected.js": "alert(1)",
		}))
		_, err := importCoverArchive(fs, "/tmp/extra.zip", false)
		if !errors.Is(err, ErrCoverArchiveInvalid) {
			t.Fatalf("got err %v, want ErrCoverArchiveInvalid (unexpected entry)", err)
		}
	})

	t.Run("zip html fails cover validation", func(t *testing.T) {
		fs := newArchiveFS(t)
		fs.files["/tmp/broken.zip"] = string(makeZip(t, map[string]string{
			"team.html": "<div>no cover markers</div>",
		}))
		_, err := importCoverArchive(fs, "/tmp/broken.zip", false)
		if !errors.Is(err, ErrCoverInvalid) {
			t.Fatalf("got err %v, want ErrCoverInvalid", err)
		}
		// Refused → nothing materialised:
		if _, exists := fs.files[onDiskCoversDir+"/team.html"]; exists {
			t.Errorf("invalid cover html was written")
		}
	})

	t.Run("nil fs", func(t *testing.T) {
		_, err := importCoverArchive(nil, "/tmp/x.zip", false)
		if err == nil {
			t.Fatalf("expected error with nil fs")
		}
	})
}

func TestImportCoverArchive_RoundTrip(t *testing.T) {
	// Export → Import on a clean fs reproduces the original cover.
	src := newArchiveFS(t)
	src.files[onDiskCoversDir+"/team.html"] = validBundlableCover
	src.files[onDiskCoversDir+"/images/banner.png"] = "banner-bytes"

	exp, err := exportCoverArchive(src, "team", "/tmp/team.zip")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(exp.MissingImages) != 0 {
		t.Fatalf("unexpected missing on export: %v", exp.MissingImages)
	}

	zipBytes, err := src.LoadFile("/tmp/team.zip")
	if err != nil {
		t.Fatalf("read exported zip: %v", err)
	}

	dst := newArchiveFS(t)
	dst.files["/tmp/team.zip"] = zipBytes

	imp, err := importCoverArchive(dst, "/tmp/team.zip", false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imp.Name != "team" {
		t.Errorf("got name %q, want team", imp.Name)
	}
	if got := dst.files[onDiskCoversDir+"/team.html"]; got != validBundlableCover {
		t.Errorf("round-trip html mismatch")
	}
	if got := dst.files[onDiskCoversDir+"/images/banner.png"]; got != "banner-bytes" {
		t.Errorf("round-trip banner mismatch (got %q)", got)
	}
}
