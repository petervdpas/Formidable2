package pdf

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// scaffoldedFS returns a memFS that's been through scaffoldCovers —
// the realistic post-boot starting state.
func scaffoldedFS(t *testing.T) *memFS {
	t.Helper()
	fs := newMemFS()
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	return fs
}

// ---------- loadDiskCover ----------

func TestLoadDiskCover_BundledLibraryNamesAllLoad(t *testing.T) {
	fs := scaffoldedFS(t)
	for _, name := range []string{"classic", "banner", "corporate"} {
		html, err := loadDiskCover(fs, name)
		if err != nil {
			t.Errorf("loadDiskCover(%q) err = %v", name, err)
			continue
		}
		if !strings.Contains(html, "data-cover-end") {
			t.Errorf("loaded %q missing sentinel", name)
		}
	}
}

func TestLoadDiskCover_EmptyName(t *testing.T) {
	fs := scaffoldedFS(t)
	_, err := loadDiskCover(fs, "")
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound", err)
	}
}

func TestLoadDiskCover_SignatureReserved(t *testing.T) {
	fs := scaffoldedFS(t)
	_, err := loadDiskCover(fs, "signature")
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound (signature is reserved)", err)
	}
}

func TestLoadDiskCover_PathSeparatorRejected(t *testing.T) {
	fs := scaffoldedFS(t)
	_, err := loadDiskCover(fs, "../escape")
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound", err)
	}
}

func TestLoadDiskCover_MissingFile(t *testing.T) {
	fs := scaffoldedFS(t)
	_, err := loadDiskCover(fs, "no-such-cover")
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound", err)
	}
}

func TestLoadDiskCover_InvalidHTMLFlagged(t *testing.T) {
	fs := newMemFS()
	// Drop a file without the magic-line header into the covers dir.
	fs.files[onDiskCoversDir+"/broken.html"] = "<section>no header</section><span data-cover-end></span>"
	_, err := loadDiskCover(fs, "broken")
	if !errors.Is(err, ErrCoverInvalid) {
		t.Errorf("err = %v, want ErrCoverInvalid", err)
	}
}

func TestLoadDiskCover_UserAddedCoverDiscoverable(t *testing.T) {
	// User drops a perfectly valid aurora.html into the dir — loader
	// must accept it as a first-class cover, no registration needed.
	fs := scaffoldedFS(t)
	fs.files[onDiskCoversDir+"/aurora.html"] = validCover
	html, err := loadDiskCover(fs, "aurora")
	if err != nil {
		t.Errorf("user-added cover not loadable: %v", err)
	}
	if !strings.Contains(html, "{{.Title}}") {
		t.Errorf("aurora HTML content did not round-trip")
	}
}

// ---------- listDiskCovers ----------

func TestListDiskCovers_AfterScaffold_ListsLibrary(t *testing.T) {
	fs := scaffoldedFS(t)
	got, err := listDiskCovers(fs)
	if err != nil {
		t.Fatalf("listDiskCovers: %v", err)
	}
	want := map[string]bool{"classic": true, "banner": true, "corporate": true}
	for w := range want {
		found := false
		for _, d := range got {
			if d.Name == w {
				found = true
				if !d.OK {
					t.Errorf("seed %q marked OK=false", w)
				}
				break
			}
		}
		if !found {
			t.Errorf("listing missing %q", w)
		}
	}
}

func TestListDiskCovers_ExcludesSignature(t *testing.T) {
	fs := scaffoldedFS(t)
	got, err := listDiskCovers(fs)
	if err != nil {
		t.Fatalf("listDiskCovers: %v", err)
	}
	for _, d := range got {
		if d.Name == "signature" {
			t.Errorf("listing should exclude signature, got %+v", d)
		}
	}
}

func TestListDiskCovers_SurfaceMagicCommentLabels(t *testing.T) {
	fs := scaffoldedFS(t)
	got, err := listDiskCovers(fs)
	if err != nil {
		t.Fatalf("listDiskCovers: %v", err)
	}
	for _, d := range got {
		if d.Name == "banner" {
			if d.Label != "Banner" {
				t.Errorf("banner Label = %q, want Banner (from magic-line)", d.Label)
			}
			if !strings.Contains(d.Description, "Hero") {
				t.Errorf("banner description did not surface: %q", d.Description)
			}
			return
		}
	}
	t.Errorf("banner entry not found")
}

func TestListDiskCovers_UserAddedCoverAutoRegistered(t *testing.T) {
	fs := scaffoldedFS(t)
	fs.files[onDiskCoversDir+"/aurora.html"] = `<!--
  formidable-cover: 1
  name: Aurora
  description: User-authored design.
-->
<section class="cover"><h1>{{.Title}}</h1></section><span data-cover-end></span>`

	got, err := listDiskCovers(fs)
	if err != nil {
		t.Fatalf("listDiskCovers: %v", err)
	}
	for _, d := range got {
		if d.Name == "aurora" {
			if !d.OK {
				t.Errorf("aurora marked invalid; got %+v", d)
			}
			if d.Label != "Aurora" {
				t.Errorf("Label = %q, want Aurora", d.Label)
			}
			return
		}
	}
	t.Errorf("user-added aurora.html not surfaced in listing")
}

func TestListDiskCovers_InvalidFileFlaggedNotDropped(t *testing.T) {
	fs := scaffoldedFS(t)
	fs.files[onDiskCoversDir+"/broken.html"] = "<section>no magic line</section>"

	got, err := listDiskCovers(fs)
	if err != nil {
		t.Fatalf("listDiskCovers: %v", err)
	}
	var seen bool
	for _, d := range got {
		if d.Name == "broken" {
			seen = true
			if d.OK {
				t.Errorf("broken.html marked OK; want false")
			}
		}
	}
	if !seen {
		t.Errorf("broken.html should appear in listing with OK=false")
	}
}

func TestListDiskCovers_EmptyDirNoError(t *testing.T) {
	fs := newMemFS()
	got, err := listDiskCovers(fs)
	if err != nil {
		t.Errorf("empty dir: err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("empty dir: got %d covers, want 0", len(got))
	}
}

// ---------- saveDiskCover ----------

func TestSaveDiskCover_ValidWrites(t *testing.T) {
	fs := newMemFS()
	if err := saveDiskCover(fs, "my-design", validCover); err != nil {
		t.Fatalf("saveDiskCover: %v", err)
	}
	if !fs.FileExists(onDiskCoversDir + "/my-design.html") {
		t.Errorf("file not written")
	}
}

func TestSaveDiskCover_InvalidRejected(t *testing.T) {
	fs := newMemFS()
	err := saveDiskCover(fs, "broken", "<section>no header</section>")
	if !errors.Is(err, ErrCoverInvalid) {
		t.Errorf("err = %v, want ErrCoverInvalid", err)
	}
	if fs.FileExists(onDiskCoversDir + "/broken.html") {
		t.Errorf("invalid file was written; SaveCover should refuse")
	}
}

func TestSaveDiskCover_ReservedNameRejected(t *testing.T) {
	fs := newMemFS()
	err := saveDiskCover(fs, "signature", validCover)
	if err == nil {
		t.Errorf("err = nil, want refusal for reserved name")
	}
}

func TestSaveDiskCover_PathSeparatorRejected(t *testing.T) {
	fs := newMemFS()
	err := saveDiskCover(fs, "../escape", validCover)
	if err == nil {
		t.Errorf("err = nil, want refusal for path-separator")
	}
}

// ---------- ResolveCoverTemplateSet against scaffolded FS ----------

func TestResolveCoverTemplateSet_OnDiskLibrary(t *testing.T) {
	fs := scaffoldedFS(t)
	cv := &CoverFM{Template: "banner"}
	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts == nil {
		t.Fatalf("ts = nil, want non-nil")
	}
	if ts.Name != "banner" {
		t.Errorf("Name = %q", ts.Name)
	}
	if !strings.Contains(ts.Cover, "cover-banner") {
		t.Errorf("Cover content lost banner marker")
	}
	if !strings.Contains(ts.Signature, "signature-block") {
		t.Errorf("Signature missing — bundled signature must be loaded from disk")
	}
}

func TestResolveCoverTemplateSet_OnDiskMissingSignatureSurfacesError(t *testing.T) {
	fs := newMemFS()
	// User has covers/banner.html but signature.html was deleted.
	fs.files[onDiskCoversDir+"/banner.html"] = validCover
	cv := &CoverFM{Template: "banner"}

	_, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if !errors.Is(err, ErrSignatureMissing) {
		t.Errorf("err = %v, want ErrSignatureMissing", err)
	}
}

func TestResolveCoverTemplateSet_TemplatePathValidates(t *testing.T) {
	fs := scaffoldedFS(t)
	// Custom user file lives next to the form's storage.
	fs.files["/storage/tpl/my-cover.html"] = validCover
	cv := &CoverFM{TemplatePath: "my-cover.html"}

	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts == nil {
		t.Fatalf("ts = nil")
	}
}

func TestResolveCoverTemplateSet_TemplatePathInvalidHTML(t *testing.T) {
	fs := scaffoldedFS(t)
	fs.files["/storage/tpl/bad.html"] = "<section>no magic line</section>"
	cv := &CoverFM{TemplatePath: "bad.html"}

	_, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if !errors.Is(err, ErrCoverInvalid) {
		t.Errorf("err = %v, want ErrCoverInvalid", err)
	}
}

func TestResolveCoverTemplateSet_NilCoverReturnsNil(t *testing.T) {
	fs := scaffoldedFS(t)
	ts, err := ResolveCoverTemplateSet(nil, "/storage/tpl", fs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts != nil {
		t.Errorf("ts = %+v, want nil for nil cover", ts)
	}
}

func TestResolveCoverTemplateSet_NeitherFieldSetReturnsNil(t *testing.T) {
	fs := scaffoldedFS(t)
	cv := &CoverFM{Title: "T"}
	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts != nil {
		t.Errorf("ts = %+v, want nil", ts)
	}
}

func TestResolveCoverTemplateSet_TemplatePathOverridesTemplate(t *testing.T) {
	fs := scaffoldedFS(t)
	// Custom file wins over named library.
	fs.files["/storage/tpl/winner.html"] = strings.Replace(validCover, "Test", "Winner", 1)
	cv := &CoverFM{TemplatePath: "winner.html", Template: "banner"}

	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(ts.Cover, "Winner") {
		t.Errorf("TemplatePath should have won; got %q", ts.Cover)
	}
}
