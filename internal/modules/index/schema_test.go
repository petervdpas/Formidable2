package index

import (
	"path/filepath"
	"testing"
)

// TestOpenIndexDB_CreatesSchema covers the first-run path: the file
// doesn't exist yet, openIndexDB creates it, runs the v1 migration,
// and stamps meta.version.
func TestOpenIndexDB_CreatesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	db, err := openIndexDB(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// meta table populated.
	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'version'`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != "1" {
		t.Errorf("version = %q, want %q", version, "1")
	}

	// All v1 tables exist.
	wantTables := []string{
		"meta", "templates", "forms", "form_tags", "images",
	}
	for _, name := range wantTables {
		var got string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
		).Scan(&got)
		if err != nil {
			t.Errorf("missing table %q: %v", name, err)
		}
	}
}

// TestOpenIndexDB_Idempotent — calling openIndexDB twice on the same
// file is a no-op for the schema; the rev counter and any user data
// must survive across re-opens.
func TestOpenIndexDB_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")

	db, err := openIndexDB(path)
	if err != nil {
		t.Fatal(err)
	}
	// Stamp a "rev" row; closing then reopening should preserve it.
	if _, err := db.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES ('rev', '7')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := openIndexDB(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	var rev string
	if err := db2.QueryRow(`SELECT value FROM meta WHERE key='rev'`).Scan(&rev); err != nil {
		t.Fatalf("read rev: %v", err)
	}
	if rev != "7" {
		t.Errorf("rev = %q, want preserved %q", rev, "7")
	}
}

// TestOpenIndexDB_FutureVersionRejected — a DB stamped with a higher
// version than this build supports is a hard error. We don't downgrade
// or wipe; the user has to use the matching app version (or delete the
// file to force a rebuild).
func TestOpenIndexDB_FutureVersionRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")

	db, err := openIndexDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE meta SET value='999' WHERE key='version'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := openIndexDB(path); err == nil {
		t.Fatal("expected error for future version, got nil")
	}
}
