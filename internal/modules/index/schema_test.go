package index

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
)

// TestOpenIndexDB_CreatesSchema covers the first-run path: the file
// doesn't exist yet, openIndexDB creates it, runs every migration in
// order, and stamps meta.version at the current schemaVersion.
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
	if version != strconv.Itoa(schemaVersion) {
		t.Errorf("version = %q, want %q", version, strconv.Itoa(schemaVersion))
	}

	// All current tables exist.
	wantTables := []string{
		"meta", "templates", "forms", "form_tags", "form_facets", "images", "form_values",
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

	// forms carries the v2 audit-identity columns and no longer carries
	// the legacy flat `author` column.
	wantCols := map[string]bool{
		"created_name":  false,
		"created_email": false,
		"updated_name":  false,
		"updated_email": false,
	}
	rows, err := db.Query(`PRAGMA table_info(forms)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "author" {
			t.Errorf("legacy `author` column still present on forms after v2")
		}
		if _, want := wantCols[name]; want {
			wantCols[name] = true
		}
	}
	for col, present := range wantCols {
		if !present {
			t.Errorf("missing column forms.%s", col)
		}
	}
}

// TestOpenIndexDB_UpgradesV1ToV2 covers the in-place migration path:
// a database stamped at v1 is reopened by a v2 binary; openIndexDB
// runs migrationV2 and lands at the current schemaVersion.
func TestOpenIndexDB_UpgradesV1ToV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	seedV1(t, path)

	db, err := openIndexDB(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'version'`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != strconv.Itoa(schemaVersion) {
		t.Errorf("version = %q, want %q", version, strconv.Itoa(schemaVersion))
	}

	// form_facets exists post-upgrade.
	var name string
	err = db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='form_facets'`,
	).Scan(&name)
	if err != nil {
		t.Errorf("form_facets not created by v2: %v", err)
	}

	// updated_name column exists post-upgrade.
	var c int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('forms') WHERE name='updated_name'`,
	).Scan(&c); err != nil {
		t.Fatalf("probe updated_name: %v", err)
	}
	if c != 1 {
		t.Errorf("updated_name column not added by v2")
	}
}

// TestOpenIndexDB_V2WipesExistingForms - v2 deletes every forms row so
// the next RescanAll re-upserts with the new audit/facet columns
// populated. Without the wipe, rows would stay with NULL identity and
// no facets until each file's mtime changed.
func TestOpenIndexDB_V2WipesExistingForms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	seedV1(t, path)

	// Seed a forms row + a template parent + a tag (so the cascade is
	// also exercised on form_tags).
	preDB, err := openV1Direct(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := preDB.Exec(
		`INSERT INTO templates (filename, name) VALUES ('t.yaml', 't')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := preDB.Exec(
		`INSERT INTO forms (template, filename, id, title, author, created, updated)
		   VALUES ('t.yaml', 'a.meta.json', 'g1', 'A', 'X', '2026-01-01', '2026-01-02')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := preDB.Exec(
		`INSERT INTO form_tags (template, filename, tag) VALUES ('t.yaml', 'a.meta.json', 'red')`,
	); err != nil {
		t.Fatal(err)
	}
	if err := preDB.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := openIndexDB(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()

	var formCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forms`).Scan(&formCount); err != nil {
		t.Fatalf("count forms: %v", err)
	}
	if formCount != 0 {
		t.Errorf("forms after v2 = %d, want 0 (wipe forces RescanAll rebuild)", formCount)
	}

	// FK cascade should have dropped the tag row too.
	var tagCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM form_tags`).Scan(&tagCount); err != nil {
		t.Fatalf("count form_tags: %v", err)
	}
	if tagCount != 0 {
		t.Errorf("form_tags after v2 = %d, want 0 (cascade)", tagCount)
	}

	// Template parent must survive - v2 only rebuilds forms, not the
	// template list.
	var tplCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM templates`).Scan(&tplCount); err != nil {
		t.Fatalf("count templates: %v", err)
	}
	if tplCount != 1 {
		t.Errorf("templates after v2 = %d, want 1 preserved", tplCount)
	}
}

// TestOpenIndexDB_UpgradesV2ToV3 covers the in-place migration path:
// a database stamped at v2 reopened by a v3 binary gains the
// form_values table and lands at the current schemaVersion.
func TestOpenIndexDB_UpgradesV2ToV3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	seedV2(t, path)

	db, err := openIndexDB(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'version'`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != strconv.Itoa(schemaVersion) {
		t.Errorf("version = %q, want %q", version, strconv.Itoa(schemaVersion))
	}

	var name string
	err = db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='form_values'`,
	).Scan(&name)
	if err != nil {
		t.Errorf("form_values not created by v3: %v", err)
	}

	// The lookup index must exist so column aggregation stays fast.
	var idx string
	err = db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_form_values_lookup'`,
	).Scan(&idx)
	if err != nil {
		t.Errorf("idx_form_values_lookup not created by v3: %v", err)
	}

	// Expected columns present.
	wantCols := map[string]bool{
		"template": false, "filename": false, "field_key": false,
		"col": false, "value_type": false, "num_value": false, "text_value": false,
	}
	rows, err := db.Query(`PRAGMA table_info(form_values)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var cname, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if _, want := wantCols[cname]; want {
			wantCols[cname] = true
		}
	}
	for col, present := range wantCols {
		if !present {
			t.Errorf("missing column form_values.%s", col)
		}
	}
}

// TestOpenIndexDB_V3WipesExistingForms - like v2, v3 deletes every
// forms row so the next RescanAll re-reads each body and populates
// form_values. Without the wipe, existing forms would carry no values
// until their mtime changed.
func TestOpenIndexDB_V3WipesExistingForms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	seedV2(t, path)

	preDB, err := openV1Direct(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := preDB.Exec(
		`INSERT INTO templates (filename, name) VALUES ('t.yaml', 't')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := preDB.Exec(
		`INSERT INTO forms (template, filename, id, title, created, updated)
		   VALUES ('t.yaml', 'a.meta.json', 'g1', 'A', '2026-01-01', '2026-01-02')`,
	); err != nil {
		t.Fatal(err)
	}
	if err := preDB.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := openIndexDB(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()

	var formCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forms`).Scan(&formCount); err != nil {
		t.Fatalf("count forms: %v", err)
	}
	if formCount != 0 {
		t.Errorf("forms after v3 = %d, want 0 (wipe forces RescanAll rebuild)", formCount)
	}

	var tplCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM templates`).Scan(&tplCount); err != nil {
		t.Fatalf("count templates: %v", err)
	}
	if tplCount != 1 {
		t.Errorf("templates after v3 = %d, want 1 preserved", tplCount)
	}
}

// seedV1 creates a fresh SQLite file at path containing only the v1
// schema, stamped at version 1. Mirrors what an older Formidable
// binary would have left on disk; used by upgrade-path tests.
func seedV1(t *testing.T, path string) {
	t.Helper()
	db, err := openV1Direct(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(migrationV1); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	if _, err := db.Exec(
		`INSERT OR REPLACE INTO meta (key, value) VALUES ('version', '1')`,
	); err != nil {
		t.Fatalf("stamp v1: %v", err)
	}
}

// seedV2 creates a fresh SQLite file at path containing the v1+v2
// schema, stamped at version 2. Mirrors what a pre-v3 Formidable
// binary would have left on disk; used by the v3 upgrade-path tests.
func seedV2(t *testing.T, path string) {
	t.Helper()
	db, err := openV1Direct(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(migrationV1); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	if _, err := db.Exec(migrationV2); err != nil {
		t.Fatalf("apply v2: %v", err)
	}
	if _, err := db.Exec(
		`INSERT OR REPLACE INTO meta (key, value) VALUES ('version', '2')`,
	); err != nil {
		t.Fatalf("stamp v2: %v", err)
	}
}

// openV1Direct opens the same DSN openIndexDB uses, without the
// migration pass - so test setup can stamp a known earlier version.
func openV1Direct(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// TestOpenIndexDB_Idempotent - calling openIndexDB twice on the same
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

// TestOpenIndexDB_FutureVersionRejected - a DB stamped with a higher
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
