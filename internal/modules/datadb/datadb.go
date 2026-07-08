// Package datadb is the queryable data substrate carried inside a Formidable
// bundle. It holds one row per collection-template record (guid, title, full
// payload as JSON) plus a full-text index, as a single SQLite image. Build
// produces the image on the authoring side with no disk write (SQLite
// Serialize); Open mounts an image read-only and in memory on the viewer side
// (a bytes-backed VFS), so a decrypted bundle's data never touches disk. The
// image is served to agents through Handler as a GET-only REST API.
package datadb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
	"modernc.org/sqlite/vfs"
)

const dbFileName = "data.db"

const schemaSQL = `
CREATE TABLE records (
  template TEXT NOT NULL,
  guid     TEXT NOT NULL,
  title    TEXT NOT NULL DEFAULT '',
  payload  TEXT NOT NULL,
  PRIMARY KEY (template, guid)
);
CREATE INDEX idx_records_template ON records(template);
CREATE VIRTUAL TABLE records_fts USING fts5(guid UNINDEXED, template UNINDEXED, title, body);
`

// Record is one collection-template record destined for the pack. Payload is the
// record's full field data (returned verbatim by the API); Text is the flattened
// searchable text (the full-text index body).
type Record struct {
	Template string
	GUID     string
	Title    string
	Payload  map[string]any
	Text     string
}

// TemplateCount is a template filename and how many records it holds.
type TemplateCount struct {
	Template string `json:"template"`
	Count    int    `json:"count"`
}

// RecordRef is a lightweight record pointer for lists and search hits.
type RecordRef struct {
	Template string `json:"template"`
	GUID     string `json:"guid"`
	Title    string `json:"title"`
}

// RecordFull is a single record with its full payload.
type RecordFull struct {
	Template string          `json:"template"`
	GUID     string          `json:"guid"`
	Title    string          `json:"title"`
	Payload  json.RawMessage `json:"payload"`
}

// Build assembles the records into a SQLite image and returns its bytes. It
// builds in an in-memory database and serializes it, so nothing is written to
// disk. Records are inserted in the given order; a duplicate (template, guid)
// is an error.
func Build(records []Record) ([]byte, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // pin to one conn: a :memory: db is per-connection

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, schemaSQL); err != nil {
		return nil, fmt.Errorf("datadb: schema: %w", err)
	}
	for _, r := range records {
		payload, err := json.Marshal(r.Payload)
		if err != nil {
			return nil, fmt.Errorf("datadb: marshal %s/%s: %w", r.Template, r.GUID, err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO records(template, guid, title, payload) VALUES(?, ?, ?, ?)`,
			r.Template, r.GUID, r.Title, string(payload)); err != nil {
			return nil, fmt.Errorf("datadb: insert %s/%s: %w", r.Template, r.GUID, err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO records_fts(guid, template, title, body) VALUES(?, ?, ?, ?)`,
			r.GUID, r.Template, r.Title, r.Text); err != nil {
			return nil, fmt.Errorf("datadb: index %s/%s: %w", r.Template, r.GUID, err)
		}
	}

	var image []byte
	if err := conn.Raw(func(dc any) error {
		s, ok := dc.(interface{ Serialize() ([]byte, error) })
		if !ok {
			return errors.New("datadb: driver does not support Serialize")
		}
		b, err := s.Serialize()
		if err != nil {
			return err
		}
		image = b
		return nil
	}); err != nil {
		return nil, err
	}
	return image, nil
}

// DB is a read-only handle over a mounted data image.
type DB struct {
	sql *sql.DB
	fs  *vfs.FS
}

// Open mounts a data image read-only and in memory. The image is served through
// a bytes-backed VFS, so it is never written to disk; Close unmounts it.
func Open(image []byte) (*DB, error) {
	if len(image) == 0 {
		return nil, errors.New("datadb: empty image")
	}
	name, fsh, err := vfs.New(memFS{data: image})
	if err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?vfs=%s&mode=ro&immutable=1", dbFileName, name)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = fsh.Close()
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		_ = fsh.Close()
		return nil, fmt.Errorf("datadb: open image: %w", err)
	}
	return &DB{sql: sqlDB, fs: fsh}, nil
}

// Close releases the database and unmounts the image.
func (d *DB) Close() error {
	err := d.sql.Close()
	if e := d.fs.Close(); err == nil {
		err = e
	}
	return err
}

// Templates lists the templates in the pack with their record counts.
func (d *DB) Templates() ([]TemplateCount, error) {
	rows, err := d.sql.Query(`SELECT template, COUNT(*) FROM records GROUP BY template ORDER BY template`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TemplateCount{}
	for rows.Next() {
		var tc TemplateCount
		if err := rows.Scan(&tc.Template, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

// Records lists the records of one template.
func (d *DB) Records(template string) ([]RecordRef, error) {
	rows, err := d.sql.Query(
		`SELECT template, guid, title FROM records WHERE template = ? ORDER BY title, guid`, template)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRefs(rows)
}

// Record returns a single record by its guid, which is globally unique.
func (d *DB) Record(guid string) (RecordFull, bool, error) {
	var r RecordFull
	var payload string
	err := d.sql.QueryRow(
		`SELECT template, guid, title, payload FROM records WHERE guid = ?`, guid).
		Scan(&r.Template, &r.GUID, &r.Title, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return RecordFull{}, false, nil
	}
	if err != nil {
		return RecordFull{}, false, err
	}
	r.Payload = json.RawMessage(payload)
	return r, true, nil
}

// Search returns records matching the full-text query. An empty query returns
// no results rather than an error.
func (d *DB) Search(query string) ([]RecordRef, error) {
	match := ftsQuery(query)
	if match == "" {
		return []RecordRef{}, nil
	}
	rows, err := d.sql.Query(
		`SELECT template, guid, title FROM records_fts WHERE records_fts MATCH ? ORDER BY rank`, match)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRefs(rows)
}

func scanRefs(rows *sql.Rows) ([]RecordRef, error) {
	out := []RecordRef{}
	for rows.Next() {
		var r RecordRef
		if err := rows.Scan(&r.Template, &r.GUID, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ftsQuery turns free user input into a safe FTS5 MATCH expression: each token
// is double-quoted (so punctuation cannot break the syntax) and given a prefix
// wildcard, then ANDed. Empty input yields "".
func ftsQuery(q string) string {
	fields := strings.Fields(q)
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, `""`)
		parts = append(parts, `"`+f+`"*`)
	}
	return strings.Join(parts, " ")
}
