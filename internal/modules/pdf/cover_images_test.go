package pdf

import (
	"errors"
	"path"
	"strings"
	"testing"
)

// Unit tests for the cover-images surface: list, save, load, delete
// the binary assets under <AppRoot>/pdf/covers/images/ that cover
// HTML files reference via {{logo}} / <img src="…">. Seed images
// (formidable.svg ships with the binary) are flagged so the
// frontend can offer Reset-to-default in place of permanent delete.

func TestListCoverImages_EmptyDir(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	got, err := m.ListCoverImages()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %d entries", len(got))
	}
}

func TestListCoverImages_ListsAllImageExtensions(t *testing.T) {
	mem := newMemFS()
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "logo.svg")] = "<svg/>"
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "banner.png")] = "\x89PNG\r\n"
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "shot.jpg")] = "\xff\xd8\xff"
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "notes.txt")] = "skip me"
	m := &Manager{store: &store{fs: mem}}

	got, err := m.ListCoverImages()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	names := []string{}
	for _, e := range got {
		names = append(names, e.Name)
	}
	wantContains := []string{"logo.svg", "banner.png", "shot.jpg"}
	for _, w := range wantContains {
		if !contains(names, w) {
			t.Errorf("want %q in %v", w, names)
		}
	}
	if contains(names, "notes.txt") {
		t.Errorf("non-image notes.txt should be filtered out, got %v", names)
	}
}

func TestListCoverImages_FlagsSeedImages(t *testing.T) {
	mem := newMemFS()
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "formidable.svg")] = "<svg/>"
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "user.png")] = "\x89PNG"
	m := &Manager{store: &store{fs: mem}}

	got, err := m.ListCoverImages()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	by := map[string]CoverImageDescriptor{}
	for _, e := range got {
		by[e.Name] = e
	}
	if !by["formidable.svg"].IsSeed {
		t.Errorf("formidable.svg should be flagged IsSeed=true")
	}
	if by["user.png"].IsSeed {
		t.Errorf("user.png should be flagged IsSeed=false")
	}
}

func TestListCoverImages_ReportsByteSize(t *testing.T) {
	mem := newMemFS()
	body := strings.Repeat("X", 256)
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "logo.png")] = body
	m := &Manager{store: &store{fs: mem}}

	got, err := m.ListCoverImages()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || got[0].Size != int64(len(body)) {
		t.Errorf("want size=%d got %+v", len(body), got)
	}
}

func TestListCoverImages_SortedAlphabetically(t *testing.T) {
	mem := newMemFS()
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "zebra.png")] = "z"
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "alpha.png")] = "a"
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "middle.png")] = "m"
	m := &Manager{store: &store{fs: mem}}

	got, _ := m.ListCoverImages()
	if len(got) != 3 || got[0].Name != "alpha.png" || got[1].Name != "middle.png" || got[2].Name != "zebra.png" {
		t.Errorf("not sorted: %+v", got)
	}
}

func TestSaveCoverImage_WritesAtomicPath(t *testing.T) {
	mem := newMemFS()
	m := &Manager{store: &store{fs: mem}}
	body := []byte("\x89PNG\r\n\x1a\nbytes")

	if err := m.SaveCoverImage("logo.png", body); err != nil {
		t.Fatalf("err = %v", err)
	}
	got, ok := mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "logo.png")]
	if !ok {
		t.Fatalf("logo.png not written")
	}
	if got != string(body) {
		t.Errorf("body mismatch: want %q got %q", body, got)
	}
}

func TestSaveCoverImage_RejectsTraversalNames(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	for _, bad := range []string{
		"../escape.png",
		"images/nested.png",
		"x/y.png",
		"..",
		"",
		".png",
		".",
	} {
		if err := m.SaveCoverImage(bad, []byte("data")); err == nil {
			t.Errorf("want error for %q", bad)
		}
	}
}

func TestSaveCoverImage_RejectsBadExtensions(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	for _, bad := range []string{
		"thing.txt",
		"thing",
		"thing.exe",
		"thing.html",
	} {
		if err := m.SaveCoverImage(bad, []byte("data")); err == nil {
			t.Errorf("want error for %q", bad)
		}
	}
}

func TestSaveCoverImage_AcceptsKnownImageExtensions(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	for _, ok := range []string{
		"a.png", "b.jpg", "c.jpeg", "d.gif", "e.svg", "f.webp",
		"upper.PNG", "mixed.SvG",
	} {
		if err := m.SaveCoverImage(ok, []byte("data")); err != nil {
			t.Errorf("want no error for %q, got %v", ok, err)
		}
	}
}

func TestSaveCoverImage_RejectsEmptyBytes(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	if err := m.SaveCoverImage("logo.png", nil); err == nil {
		t.Errorf("want error for nil bytes")
	}
	if err := m.SaveCoverImage("logo.png", []byte{}); err == nil {
		t.Errorf("want error for empty bytes")
	}
}

func TestSaveCoverImage_BubblesWriteErr(t *testing.T) {
	mem := newMemFS()
	mem.saveErr = errors.New("disk full")
	m := &Manager{store: &store{fs: mem}}
	if err := m.SaveCoverImage("logo.png", []byte("data")); err == nil {
		t.Errorf("want error when fs.SaveFile fails")
	}
}

func TestLoadCoverImage_RoundTripsBytes(t *testing.T) {
	mem := newMemFS()
	body := []byte("\x89PNG\r\n\x1a\nbinary")
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "logo.png")] = string(body)
	m := &Manager{store: &store{fs: mem}}

	got, err := m.LoadCoverImage("logo.png")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("body mismatch")
	}
}

func TestLoadCoverImage_MissingReturnsError(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	if _, err := m.LoadCoverImage("ghost.png"); err == nil {
		t.Errorf("want error for missing")
	}
}

func TestLoadCoverImage_RejectsTraversal(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	if _, err := m.LoadCoverImage("../escape.png"); err == nil {
		t.Errorf("want error for traversal")
	}
}

func TestDeleteCoverImage_RemovesUserImage(t *testing.T) {
	mem := newMemFS()
	p := path.Join(onDiskCoversDir, coverImagesSubdir, "user.png")
	mem.files[p] = "data"
	m := &Manager{store: &store{fs: mem}}

	if err := m.DeleteCoverImage("user.png"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, exists := mem.files[p]; exists {
		t.Errorf("user.png still on disk after delete")
	}
}

func TestDeleteCoverImage_MissingIsNoOp(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	if err := m.DeleteCoverImage("ghost.png"); err != nil {
		t.Errorf("delete missing should be no-op, got %v", err)
	}
}

func TestDeleteCoverImage_RejectsTraversal(t *testing.T) {
	m := &Manager{store: &store{fs: newMemFS()}}
	if err := m.DeleteCoverImage("../escape.png"); err == nil {
		t.Errorf("want error for traversal")
	}
}

func TestSeedCoverImages_IncludesFormidableSVG(t *testing.T) {
	seeds := seedCoverImageNames()
	if !contains(seeds, "formidable.svg") {
		t.Errorf("expected formidable.svg in seed set, got %v", seeds)
	}
}

func TestService_SaveCoverImage_DecodesBase64(t *testing.T) {
	mem := newMemFS()
	svc := NewService(&Manager{store: &store{fs: mem}})
	if err := svc.SaveCoverImage("logo.png", "aGVsbG8="); err != nil {
		t.Fatalf("err = %v", err)
	}
	got := mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "logo.png")]
	if got != "hello" {
		t.Errorf("body = %q, want %q", got, "hello")
	}
}

func TestService_SaveCoverImage_StripsDataURLPrefix(t *testing.T) {
	mem := newMemFS()
	svc := NewService(&Manager{store: &store{fs: mem}})
	if err := svc.SaveCoverImage("logo.png", "data:image/png;base64,aGVsbG8="); err != nil {
		t.Fatalf("err = %v", err)
	}
	got := mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "logo.png")]
	if got != "hello" {
		t.Errorf("body = %q, want %q", got, "hello")
	}
}

func TestService_LoadCoverImage_EncodesBase64(t *testing.T) {
	mem := newMemFS()
	mem.files[path.Join(onDiskCoversDir, coverImagesSubdir, "logo.png")] = "hello"
	svc := NewService(&Manager{store: &store{fs: mem}})
	got, err := svc.LoadCoverImage("logo.png")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "aGVsbG8=" {
		t.Errorf("base64 = %q, want %q", got, "aGVsbG8=")
	}
}

func TestService_SaveCoverImage_RejectsMalformedBase64(t *testing.T) {
	svc := NewService(&Manager{store: &store{fs: newMemFS()}})
	if err := svc.SaveCoverImage("logo.png", "@@@not-base64@@@"); err == nil {
		t.Errorf("want error for malformed base64")
	}
}
