// Package index owns the per-profile SQLite index backing the wiki and API. It is a cache of disk state;
// the filesystem is canonical and Reconcile brings the index back into agreement. One DB per profile.
package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	// Pure-Go SQLite driver (no CGO), registered as the "sqlite" driver on import.
	_ "modernc.org/sqlite"
)

// schemaVersion is the version this binary writes; a higher stamped version is rejected (no downgrade).
const schemaVersion = 5

// migrations apply in order, each bumping meta.version on success; index 0 is an unused placeholder.
var migrations = []string{
	"", // v0 placeholder, never applied
	migrationV1,
	migrationV2,
	migrationV3,
	migrationV4,
	migrationV5,
}

// migrationV1 is the initial schema.
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

// migrationV2 lands facets: split audit name/email, form_facets (so ?facet.<k>=L becomes one JOIN, not N reads),
// and DELETE FROM forms so RescanAll rebuilds every row populated (safe: the index is a derived view).
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

// migrationV3 lands statistics: form_values (one row per scalar field, one per table cell) so charts run
// SUM/AVG/GROUP BY over an index instead of scanning bodies. col is NULL for scalars, 0..N for table columns;
// num_value holds the parsed number (epoch for dates), text_value the display string. DELETE FROM forms as in v2.
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

// migrationV4 lands full-text search: form_search (flattened title+body) plus form_fts, an FTS5
// external-content index over it (no prose duplication). Three triggers keep the FTS shadow in lock-step
// with form_search; the reconciler drives it via direct DELETE/INSERT (not a bare cascade) so triggers always fire.
const migrationV4 = `
CREATE TABLE form_search (
    template TEXT NOT NULL,
    filename TEXT NOT NULL,
    title    TEXT,
    body     TEXT,
    PRIMARY KEY (template, filename),
    FOREIGN KEY (template, filename) REFERENCES forms(template, filename) ON DELETE CASCADE
);

CREATE VIRTUAL TABLE form_fts USING fts5(
    title,
    body,
    content='form_search',
    content_rowid='rowid',
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER form_search_ai AFTER INSERT ON form_search BEGIN
    INSERT INTO form_fts(rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;
CREATE TRIGGER form_search_ad AFTER DELETE ON form_search BEGIN
    INSERT INTO form_fts(form_fts, rowid, title, body) VALUES ('delete', old.rowid, old.title, old.body);
END;
CREATE TRIGGER form_search_au AFTER UPDATE ON form_search BEGIN
    INSERT INTO form_fts(form_fts, rowid, title, body) VALUES ('delete', old.rowid, old.title, old.body);
    INSERT INTO form_fts(rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;

DELETE FROM forms;
`

// migrationV5 records whether a template is a presentation (slide deck). It is a
// data-facing exclusion flag: presentation templates are collections but their
// records are slides, so the api/query/datacore/stat surfaces skip them. An
// additive templates column backfilled on the next reconcile (no forms rebuild).
const migrationV5 = `
ALTER TABLE templates ADD COLUMN presentation INTEGER NOT NULL DEFAULT 0;
`

// openIndexDB opens (or creates) the SQLite file and migrates to schemaVersion. FKs are enabled so the
// ON DELETE CASCADE rules fire; WAL for concurrent readers.
func openIndexDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("index: ensure dir: %w", err)
	}

	// The modernc driver doesn't carry per-connection pragmas across the pool, so cap at one connection
	// (SetMaxOpenConns below) for predictable foreign_keys + WAL semantics.
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

// migrate applies the missing migrations up to schemaVersion; a higher-than-expected version errors.
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

// readVersion returns 0 for a fresh DB (no meta table), or the stamped value.
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

// applyMigration runs one version's migration in a transaction and stamps meta.version on success.
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
