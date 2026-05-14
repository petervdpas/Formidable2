package gigot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ── wipeManagedContent ──────────────────────────────────────────────

func TestWipeManagedContent_RejectsBlankContext(t *testing.T) {
	if err := wipeManagedContent(""); !errors.Is(err, ErrMissingContext) {
		t.Fatalf("want ErrMissingContext, got %v", err)
	}
}

func TestWipeManagedContent_MissingFolderIsNoop(t *testing.T) {
	if err := wipeManagedContent(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatalf("missing folder should be a no-op, got %v", err)
	}
}

func TestWipeManagedContent_RemovesTemplatesAndStorageTrees(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/basic.yaml", "old\n")
	writeFile(t, dir, "templates/notes.yaml", "old\n")
	writeFile(t, dir, "storage/addresses/oak.meta.json", "{}")
	writeFile(t, dir, "storage/addresses/images/photo.jpg", "binary")
	writeFile(t, dir, "storage/notes/x.meta.json", "{}")

	if err := wipeManagedContent(dir); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"templates", "storage"} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("%s/ should be gone, stat err = %v", p, err)
		}
	}
}

func TestWipeManagedContent_RemovesRootAllowlistFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "old readme")
	writeFile(t, dir, ".gitignore", ".formidable/sync.json")

	if err := wipeManagedContent(dir); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"README.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("%s should be gone, stat err = %v", p, err)
		}
	}
}

func TestWipeManagedContent_RemovesLedger(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".formidable/sync.json", `{"version":"v1"}`)

	if err := wipeManagedContent(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(TrackRecordPath(dir)); !os.IsNotExist(err) {
		t.Errorf(".formidable/sync.json should be gone, stat err = %v", err)
	}
}

func TestWipeManagedContent_PreservesContextFolderItself(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/basic.yaml", "x")
	if err := wipeManagedContent(dir); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("context folder itself should remain, info=%v err=%v", info, err)
	}
}

func TestWipeManagedContent_PreservesUserOwnedAndDotFormidableDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/basic.yaml", "x")
	writeFile(t, dir, "storage/x/y.meta.json", "{}")
	writeFile(t, dir, "notes.txt", "user-owned, must survive")
	writeFile(t, dir, "private/log.txt", "user-owned subtree, must survive")
	// Other files inside .formidable/ (scaffold marker, future things)
	// must survive — only the ledger is gigot's to delete.
	writeFile(t, dir, ".formidable/context.json", `{"version":1}`)
	writeFile(t, dir, ".formidable/sync.json", `{"version":"v1"}`)

	if err := wipeManagedContent(dir); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"notes.txt", "private/log.txt", ".formidable/context.json"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("preserved %q lost: %v", p, err)
		}
	}
	if _, err := os.Stat(TrackRecordPath(dir)); !os.IsNotExist(err) {
		t.Errorf("ledger should be gone, stat err = %v", err)
	}
}

func TestWipeManagedContent_IdempotentOnAlreadyEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := wipeManagedContent(dir); err != nil {
		t.Fatal(err)
	}
	if err := wipeManagedContent(dir); err != nil {
		t.Fatal(err)
	}
}
