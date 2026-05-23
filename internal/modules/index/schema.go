// Package index owns the per-profile SQLite index that backs the wiki
// HTTP server and the future API. It's a *cache* of disk state - the
// canonical source of truth is always the file system. Reconcile
// (RescanAll / RescanTemplate / RescanForm) brings the index back into
// agreement with disk; the index never asserts authority over it.
//
// One file per profile lives at <AppRoot>/index/<profile-stem>.db.
// Profile switch closes the current handle and opens the new one.
package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	// Pure-Go SQLite driver - no CGO, registered as the "sqlite" driver
	// for database/sql on import.
	_ "modernc.org/sqlite"
)

// schemaVersion is the version this binary writes and accepts. A DB
// file stamped with a higher version is rejected (we don't downgrade);
// a lower version triggers the matching forward migration.
const schemaVersion = 3

// migrations are applied in order; each one bumps meta.version when it
// returns successfully. Index 0 is unused so the slice index lines up
// with the version it produces.
var migrations = []string{
	"", // v0 - placeholder, never applied
	migrationV1,
	migrationV2,
	migrationV3,
}

// migrationV1 is the initial schema. Lives as a Go string so the file
// is self-contained (no embed FS needed for one tiny migration).
const migrationV1 = `
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);

CREATE TABLE templates (
    filename              TEXT PRIMARY KEY,
    name                  TEXT,
    item_field            TEXT,
    guid_field            TEXT,
    tags_field            TEXT,
    has_markdown_template INTEGER NOT NULL DEFAULT 0,
    enable_collection     INTEGER NOT NULL DEFAULT 0,
    rev                   INTEGER NOT NULL DEFAULT 0,
    mtime                 INTEGER NOT NULL DEFAULT 0,
    size                  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE forms (
    template          TEXT NOT NULL,
    filename          TEXT NOT NULL,
    id                TEXT,
    title             TEXT,
    fm_title          TEXT,
    author            TEXT,
    created           TEXT,
    updated           TEXT,
    expression_items  TEXT,
    rev               INTEGER NOT NULL DEFAULT 0,
    mtime             INTEGER NOT NULL DEFAULT 0,
    size              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (template, filename),
    FOREIGN KEY (template) REFERENCES templates(filename) ON DELETE CASCADE
);
CREATE INDEX idx_forms_updated ON forms(template, updated DESC);
CREATE INDEX idx_forms_id      ON forms(id) WHERE id IS NOT NULL;

CREATE TABLE form_tags (
    template TEXT NOT NULL,
    filename TEXT NOT NULL,
    tag      TEXT NOT NULL,
    PRIMARY KEY (template, filename, tag),
    FOREIGN KEY (template, filename) REFERENCES forms(template, filename) ON DELETE CASCADE
);
CREATE INDEX idx_form_tags_tag ON form_tags(tag);

CREATE TABLE images (
    template TEXT NOT NULL,
    filename TEXT NOT NULL,
    mtime    INTEGER NOT NULL DEFAULT 0,
    size     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (template, filename),
    FOREIGN KEY (template) REFERENCES templates(filename) ON DELETE CASCADE
);
`

// migrationV2 lands the facets feature on the index side. It (a)
// replaces the flat `author` column on forms with split name/email
// for both Created and Updated audit entries, (b) creates form_facets
// - one row per facet per form, mirroring form_tags - so the REST
// `?facet.<k>=L` filter can move from N disk reads to one SQL JOIN,
// and (c) DELETE FROM forms so the boot-time RescanAll rebuilds every
// row with the new audit + facet columns populated. The wipe is safe
// because the index is a derived view of disk; the cascade on
// form_tags fires automatically.
const migrationV2 = `
ALTER TABLE forms ADD COLUMN created_name  TEXT;
ALTER TABLE forms ADD COLUMN created_email TEXT;
ALTER TABLE forms ADD COLUMN updated_name  TEXT;
ALTER TABLE forms ADD COLUMN updated_email TEXT;
ALTER TABLE forms DROP COLUMN author;

CREATE TABLE form_facets (
    template  TEXT NOT NULL,
    filename  TEXT NOT NULL,
    facet_key TEXT NOT NULL,
    set_flag  INTEGER NOT NULL DEFAULT 0,
    selected  TEXT,
    PRIMARY KEY (template, filename, facet_key),
    FOREIGN KEY (template, filename) REFERENCES forms(template, filename) ON DELETE CASCADE
);
CREATE INDEX idx_form_facets_lookup
    ON form_facets(template, facet_key, selected)
    WHERE set_flag = 1;

DELETE FROM forms;
`

// migrationV3 lands the statistics feature on the index side. It adds
// form_values - one row per aggregatable scalar field and one row per
// table-field cell - so charts can run SUM / AVG / GROUP BY over an
// indexed column instead of scanning .meta.json bodies at query time.
// Like form_facets, this is a derived cache: reconcile reads each body
// once (it already does, for title/tags/facets) and materialises the
// values here. col is NULL for scalar fields, 0..N for table columns.
// value_type tags the cell so date grouping / numeric range stats can
// pick the right column without re-reading the template. num_value
// holds the parsed number (or epoch seconds for a date); text_value
// holds the raw/display string (ISO "YYYY-MM-DD" for a date) for
// distribution group-by. DELETE FROM forms forces RescanAll to rebuild
// every row with values populated - same rationale as v2.
const migrationV3 = `
CREATE TABLE form_values (
    template   TEXT NOT NULL,
    filename   TEXT NOT NULL,
    field_key  TEXT NOT NULL,
    col        INTEGER,
    value_type TEXT,
    num_value  REAL,
    text_value TEXT,
    FOREIGN KEY (template, filename) REFERENCES forms(template, filename) ON DELETE CASCADE
);
CREATE INDEX idx_form_values_lookup ON form_values(template, field_key, col);

DELETE FROM forms;
`

// openIndexDB opens (or creates) the SQLite file at path and brings
// its schema up to schemaVersion. Foreign keys are enabled so the
// ON DELETE CASCADE rules in v1 actually fire. WAL mode for slightly
// better concurrent-reader behavior - not strictly required since we
// own all writes through one Manager, but it's cheap insurance.
func openIndexDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("index: ensure dir: %w", err)
	}

	// _pragma=foreign_keys(1) enables FK enforcement for THIS connection;
	// the modernc driver doesn't carry it across pooled connections, so
	// we keep the pool to one connection (see SetMaxOpenConns below) for
	// predictable semantics. Same reason for journal_mode=wal.
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("index: open %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// migrate brings the DB up to schemaVersion. New file → all migrations
// from v1 onward. Existing file → only the missing ones. Higher-than-
// expected version → error.
func migrate(db *sql.DB) error {
	current, err := readVersion(db)
	if err != nil {
		return err
	}
	if current > schemaVersion {
		return fmt.Errorf("index: db version %d > supported %d (use a newer build or delete the index)", current, schemaVersion)
	}
	for v := current + 1; v <= schemaVersion; v++ {
		if err := applyMigration(db, v); err != nil {
			return fmt.Errorf("index: migrate to v%d: %w", v, err)
		}
	}
	return nil
}

// readVersion returns 0 for a fresh DB (no meta table yet), or the
// stamped value otherwise.
func readVersion(db *sql.DB) (int, error) {
	var hasMeta int
	err := db.QueryRow(
		`SELECT 1 FROM sqlite_master WHERE type='table' AND name='meta'`,
	).Scan(&hasMeta)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("index: probe meta: %w", err)
	}

	var raw string
	err = db.QueryRow(`SELECT value FROM meta WHERE key='version'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("index: read version: %w", err)
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("index: parse version %q: %w", raw, err)
	}
	return v, nil
}

// applyMigration runs the migration at the given version inside a
// single transaction and stamps meta.version on success.
func applyMigration(db *sql.DB, version int) error {
	if version <= 0 || version >= len(migrations) {
		return fmt.Errorf("no migration registered for v%d", version)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Commit clears it
	if _, err := tx.Exec(migrations[version]); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO meta (key, value) VALUES ('version', ?)`,
		strconv.Itoa(version),
	); err != nil {
		return err
	}
	return tx.Commit()
}
