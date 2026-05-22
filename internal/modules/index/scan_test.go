package index

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// makeFile creates a file with given content and mtime (unix nano).
// Returns the absolute path. mtime nanos == 0 means "leave as-is".
func makeFile(t *testing.T, path, content string, mtimeUnix int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if mtimeUnix > 0 {
		if err := os.Chtimes(path, atimeNoop, secsToTime(mtimeUnix)); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
}

func entryNames(es []FileEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Filename
	}
	sort.Strings(out)
	return out
}

func TestScanDisk_FullTree(t *testing.T) {
	root := t.TempDir()

	// Templates directory.
	makeFile(t, filepath.Join(root, "templates", "basic.yaml"), "name: Basic\n", 1700_000_001)
	makeFile(t, filepath.Join(root, "templates", "looper.yaml"), "name: Looper\n", 1700_000_002)
	// Non-yaml files in templates/ should be ignored.
	makeFile(t, filepath.Join(root, "templates", "README.md"), "ignore me", 0)

	// Per-template storage with meta.json files.
	makeFile(t, filepath.Join(root, "storage", "basic", "one.meta.json"), `{"meta":{}}`, 1700_000_003)
	makeFile(t, filepath.Join(root, "storage", "basic", "two.meta.json"), `{"meta":{}}`, 1700_000_004)
	// Junk JSON without .meta.json suffix should be ignored.
	makeFile(t, filepath.Join(root, "storage", "basic", "junk.json"), "ignore", 0)

	// Per-template images.
	makeFile(t, filepath.Join(root, "storage", "basic", "images", "logo.png"), "PNG", 1700_000_005)
	makeFile(t, filepath.Join(root, "storage", "basic", "images", "photo.jpg"), "JPG", 1700_000_006)

	// Storage folder for a template that no longer has a yaml - these
	// orphan storage dirs are still returned so the reconciler can
	// notice and clean them up later (or leave them alone).
	makeFile(t, filepath.Join(root, "storage", "ghost", "x.meta.json"), `{}`, 1700_000_007)

	got, err := scanDisk(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	wantTemplates := []string{"basic.yaml", "looper.yaml"}
	if names := entryNames(got.Templates); !equalStrings(names, wantTemplates) {
		t.Errorf("templates = %v, want %v", names, wantTemplates)
	}

	wantBasicForms := []string{"one.meta.json", "two.meta.json"}
	if names := entryNames(got.Forms["basic"]); !equalStrings(names, wantBasicForms) {
		t.Errorf("basic forms = %v, want %v", names, wantBasicForms)
	}

	wantBasicImages := []string{"logo.png", "photo.jpg"}
	if names := entryNames(got.Images["basic"]); !equalStrings(names, wantBasicImages) {
		t.Errorf("basic images = %v, want %v", names, wantBasicImages)
	}

	// Orphan storage dir is reported so reconcile can act on it.
	if names := entryNames(got.Forms["ghost"]); !equalStrings(names, []string{"x.meta.json"}) {
		t.Errorf("ghost forms = %v, want [x.meta.json]", names)
	}
}

func TestScanDisk_EmptyTree(t *testing.T) {
	// Both subdirs missing - scan must succeed with empty results.
	root := t.TempDir()
	got, err := scanDisk(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got.Templates) != 0 {
		t.Errorf("templates = %v, want empty", got.Templates)
	}
	if len(got.Forms) != 0 {
		t.Errorf("forms = %v, want empty", got.Forms)
	}
	if len(got.Images) != 0 {
		t.Errorf("images = %v, want empty", got.Images)
	}
}

func TestScanDisk_TemplatesOnlyNoStorage(t *testing.T) {
	root := t.TempDir()
	makeFile(t, filepath.Join(root, "templates", "basic.yaml"), "name: x", 1700_000_001)

	got, err := scanDisk(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got.Templates) != 1 {
		t.Errorf("templates count = %d, want 1", len(got.Templates))
	}
	if len(got.Forms) != 0 || len(got.Images) != 0 {
		t.Errorf("forms/images expected empty, got %v / %v", got.Forms, got.Images)
	}
}

func TestDiffEntries_Added(t *testing.T) {
	disk := []FileEntry{{Filename: "a", Mtime: 1, Size: 10}, {Filename: "b", Mtime: 2, Size: 20}}
	idx := []FileEntry{}
	d := diffEntries(disk, idx)
	if names := entryNames(d.Added); !equalStrings(names, []string{"a", "b"}) {
		t.Errorf("added = %v", names)
	}
	if len(d.Changed) != 0 || len(d.Removed) != 0 {
		t.Errorf("changed=%v removed=%v want empty", d.Changed, d.Removed)
	}
}

func TestDiffEntries_Removed(t *testing.T) {
	disk := []FileEntry{}
	idx := []FileEntry{{Filename: "gone", Mtime: 1, Size: 1}}
	d := diffEntries(disk, idx)
	if !equalStrings(d.Removed, []string{"gone"}) {
		t.Errorf("removed = %v, want [gone]", d.Removed)
	}
	if len(d.Added) != 0 || len(d.Changed) != 0 {
		t.Errorf("expected only removed, got added=%v changed=%v", d.Added, d.Changed)
	}
}

func TestDiffEntries_Changed_MtimeDiffers(t *testing.T) {
	disk := []FileEntry{{Filename: "x", Mtime: 200, Size: 10}}
	idx := []FileEntry{{Filename: "x", Mtime: 100, Size: 10}}
	d := diffEntries(disk, idx)
	if names := entryNames(d.Changed); !equalStrings(names, []string{"x"}) {
		t.Errorf("changed = %v, want [x]", names)
	}
}

func TestDiffEntries_Changed_SizeDiffersDespiteEqualMtime(t *testing.T) {
	// Edge case: file rewritten so fast that mtime granularity didn't
	// notice. Size catches it.
	disk := []FileEntry{{Filename: "x", Mtime: 100, Size: 10}}
	idx := []FileEntry{{Filename: "x", Mtime: 100, Size: 999}}
	d := diffEntries(disk, idx)
	if names := entryNames(d.Changed); !equalStrings(names, []string{"x"}) {
		t.Errorf("changed = %v, want [x]", names)
	}
}

func TestDiffEntries_NoChange(t *testing.T) {
	disk := []FileEntry{{Filename: "x", Mtime: 100, Size: 10}}
	idx := []FileEntry{{Filename: "x", Mtime: 100, Size: 10}}
	d := diffEntries(disk, idx)
	if len(d.Added)+len(d.Changed)+len(d.Removed) != 0 {
		t.Errorf("expected no diff, got %+v", d)
	}
}

func TestDiffEntries_Combined(t *testing.T) {
	disk := []FileEntry{
		{Filename: "keep", Mtime: 100, Size: 10},
		{Filename: "modified", Mtime: 200, Size: 20},
		{Filename: "added", Mtime: 300, Size: 30},
	}
	idx := []FileEntry{
		{Filename: "keep", Mtime: 100, Size: 10},
		{Filename: "modified", Mtime: 100, Size: 20},
		{Filename: "removed", Mtime: 50, Size: 5},
	}
	d := diffEntries(disk, idx)
	if names := entryNames(d.Added); !equalStrings(names, []string{"added"}) {
		t.Errorf("added = %v", names)
	}
	if names := entryNames(d.Changed); !equalStrings(names, []string{"modified"}) {
		t.Errorf("changed = %v", names)
	}
	if !equalStrings(d.Removed, []string{"removed"}) {
		t.Errorf("removed = %v", d.Removed)
	}
}
