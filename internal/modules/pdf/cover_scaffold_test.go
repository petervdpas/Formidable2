package pdf

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestScaffoldCovers_WritesSeedsToEmptyFS(t *testing.T) {
	fs := newMemFS()
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("scaffoldCovers: %v", err)
	}
	for _, name := range []string{"classic", "banner", "corporate", "signature"} {
		p := onDiskCoversDir + "/" + name + ".html"
		if !fs.FileExists(p) {
			t.Errorf("seed not scaffolded at %q", p)
		}
	}
}

func TestScaffoldCovers_LeavesExistingFilesAlone(t *testing.T) {
	fs := newMemFS()
	// Pre-seed a user-edited file with distinctive content.
	userBody := "<!-- USER EDITED -->\nrest of body"
	fs.files[onDiskCoversDir+"/classic.html"] = userBody

	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("scaffoldCovers: %v", err)
	}
	got := fs.files[onDiskCoversDir+"/classic.html"]
	if got != userBody {
		t.Errorf("user-edited classic.html was clobbered; got %q", got)
	}
	// Other seeds should still get written.
	if !fs.FileExists(onDiskCoversDir + "/banner.html") {
		t.Errorf("banner.html should have been scaffolded")
	}
}

func TestScaffoldCovers_IdempotentOnRepeatedRuns(t *testing.T) {
	fs := newMemFS()
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	classicV1 := fs.files[onDiskCoversDir+"/classic.html"]

	// Second pass must not overwrite.
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("second scaffold: %v", err)
	}
	if got := fs.files[onDiskCoversDir+"/classic.html"]; got != classicV1 {
		t.Errorf("second scaffold modified classic.html")
	}
}

func TestScaffoldCovers_RewritesDeletedFiles(t *testing.T) {
	fs := newMemFS()
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	// User deletes one seed.
	delete(fs.files, onDiskCoversDir+"/banner.html")

	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("second scaffold: %v", err)
	}
	if !fs.FileExists(onDiskCoversDir + "/banner.html") {
		t.Errorf("banner.html should have been rescaffolded after deletion")
	}
}

func TestScaffoldCovers_NilFSIsNoOp(t *testing.T) {
	if err := scaffoldCovers(nil, slog.Default()); err != nil {
		t.Errorf("nil fs: err = %v, want nil (no-op)", err)
	}
}

func TestScaffoldCovers_SaveErrorLoggedNotFatal(t *testing.T) {
	fs := newMemFS()
	fs.saveErr = errors.New("disk full")

	// Should not return a fatal error — partial failure is logged and
	// the rest of the seeds are attempted (same fate).
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Errorf("scaffold returned err = %v, want nil (per-file failures are non-fatal)", err)
	}
	// Nothing should have made it onto disk.
	for k := range fs.files {
		if strings.HasPrefix(k, onDiskCoversDir+"/") {
			t.Errorf("unexpected on-disk file after all-failing scaffold: %q", k)
		}
	}
}

func TestScaffoldCovers_ScaffoldsImagesSubdir(t *testing.T) {
	fs := newMemFS()
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	p := onDiskCoversDir + "/images/formidable.svg"
	if !fs.FileExists(p) {
		t.Errorf("default logo not scaffolded at %q", p)
	}
	got := fs.files[p]
	if len(got) < 100 || got[:5] != "<?xml" {
		t.Errorf("scaffolded svg looks malformed; len=%d head=%q", len(got), got[:min(5, len(got))])
	}
}

func TestScaffoldCovers_ScaffoldedFilesValidateOK(t *testing.T) {
	fs := newMemFS()
	if err := scaffoldCovers(fs, slog.Default()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	// Every scaffolded cover (except signature) must pass ValidateCover.
	for _, name := range []string{"classic", "banner", "corporate"} {
		content := fs.files[onDiskCoversDir+"/"+name+".html"]
		v := ValidateCover(content)
		if !v.OK {
			t.Errorf("scaffolded %q fails validation: %+v", name, v.Issues)
		}
	}
}
