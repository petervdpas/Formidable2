package index

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Manager owns the SQLite handle for one profile's index. It exposes
// fast, read-shaped methods consumed by dataprovider (and tests).
// Writes happen through Reconcile; the manager doesn't expose direct
// SQL to keep the contract narrow.
type Manager struct {
	db *sql.DB
}

// NewManager opens (or creates) the index DB at path and brings its
// schema up to date.
func NewManager(path string) (*Manager, error) {
	db, err := openIndexDB(path)
	if err != nil {
		return nil, err
	}
	return &Manager{db: db}, nil
}

// DB exposes the underlying handle so the same package's reconciler
// can run write transactions without a circular import. Not part of
// the public API consumed by dataprovider - call Reconcile instead.
func (m *Manager) DB() *sql.DB { return m.db }

// Close releases the underlying handle. Safe to call once; further
// reads will error from a closed connection.
func (m *Manager) Close() error { return m.db.Close() }

// Rev returns the monotonic revision counter, bumped once per
// successful Reconcile batch. The wiki / API uses this as an ETag.
// A fresh DB returns 0.
func (m *Manager) Rev() (int64, error) {
	var raw string
	err := m.db.QueryRow(`SELECT value FROM meta WHERE key='rev'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("index: read rev: %w", err)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("index: parse rev %q: %w", raw, err)
	}
	return v, nil
}

// ListTemplates returns all indexed templates ordered by filename ASC.
// The wiki landing page calls this directly.
func (m *Manager) ListTemplates() ([]TemplateRow, error) {
	rows, err := m.db.Query(`
		SELECT filename, name, item_field, guid_field, tags_field,
		       has_markdown_template, enable_collection, mtime, size
		FROM templates
		ORDER BY filename ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("index: list templates: %w", err)
	}
	defer rows.Close()

	var out []TemplateRow
	for rows.Next() {
		var r TemplateRow
		var hasMD, enableCol int
		if err := rows.Scan(
			&r.Filename, &r.Name, &r.ItemField, &r.GuidField, &r.TagsField,
			&hasMD, &enableCol, &r.Mtime, &r.Size,
		); err != nil {
			return nil, fmt.Errorf("index: scan template: %w", err)
		}
		r.HasMarkdownTemplate = hasMD != 0
		r.EnableCollection = enableCol != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListForms returns forms for one template, optionally filtered by
// tags (AND semantics) and shaped by QueryOpts.
func (m *Manager) ListForms(template string, opts QueryOpts) ([]FormRow, error) {
	return m.queryForms(formsByTemplateSQL{template: template}, opts)
}

// ListByTags returns forms across ALL templates that own every listed
// tag (AND semantics). Same shape and tag-handling as ListForms; the
// query just spans templates.
func (m *Manager) ListByTags(tags []string) ([]FormRow, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	return m.queryForms(formsByTagsSQL{}, QueryOpts{Tags: tags})
}

// GetForm fetches one row by composite key. Returns (nil, false, nil)
// when not present so the caller can distinguish "absent" from "error".
func (m *Manager) GetForm(template, datafile string) (*FormRow, bool, error) {
	rows, err := m.queryForms(formsByKeySQL{template: template, filename: datafile}, QueryOpts{})
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	r := rows[0]
	return &r, true, nil
}

// ── internals ────────────────────────────────────────────────────────

// formsQuerySpec is the small set of "where shapes" the read API
// needs. Each implementation contributes a WHERE clause and the
// matching args; QueryOpts adds tag filtering, ordering, and limit on
// top.
type formsQuerySpec interface {
	where() (sql string, args []any)
}

type formsByTemplateSQL struct{ template string }

func (s formsByTemplateSQL) where() (string, []any) {
	return "f.template = ?", []any{s.template}
}

type formsByKeySQL struct{ template, filename string }

func (s formsByKeySQL) where() (string, []any) {
	return "f.template = ? AND f.filename = ?", []any{s.template, s.filename}
}

type formsByTagsSQL struct{}

func (formsByTagsSQL) where() (string, []any) { return "1 = 1", nil }

// queryForms is the shared workhorse: builds SELECT … FROM forms with
// an inner JOIN that re-attaches tags via GROUP_CONCAT, applies the
// spec's WHERE, the optional tags AND-filter, the OrderBy and limit/
// offset, then materializes []FormRow with tags split back into a
// slice.
//
// US (\x1f) is used as the GROUP_CONCAT separator - tags shouldn't
// contain comma but they certainly never contain US, so this is a
// safer choice for round-trip than the default ','.
func (m *Manager) queryForms(spec formsQuerySpec, opts QueryOpts) ([]FormRow, error) {
	whereClause, whereArgs := spec.where()

	var tagAndClause string
	tagAndArgs := []any{}
	if len(opts.Tags) > 0 {
		// Subquery: forms that have at least every requested tag.
		// COUNT(DISTINCT tag) = N pattern works because primary key on
		// (template,filename,tag) prevents duplicates.
		placeholders := strings.Repeat("?,", len(opts.Tags))
		placeholders = strings.TrimRight(placeholders, ",")
		tagAndClause = fmt.Sprintf(` AND (f.template, f.filename) IN (
			SELECT template, filename
			FROM form_tags
			WHERE tag IN (%s)
			GROUP BY template, filename
			HAVING COUNT(DISTINCT tag) = ?
		)`, placeholders)
		for _, t := range opts.Tags {
			tagAndArgs = append(tagAndArgs, t)
		}
		tagAndArgs = append(tagAndArgs, len(opts.Tags))
	}

	q := fmt.Sprintf(`
		SELECT f.template, f.filename, f.id, f.title, f.fm_title,
		       f.created, f.created_name, f.created_email,
		       f.updated, f.updated_name, f.updated_email,
		       f.expression_items, f.mtime, f.size,
		       COALESCE(GROUP_CONCAT(t.tag, char(31)), '') AS tags
		FROM forms f
		LEFT JOIN form_tags t ON t.template = f.template AND t.filename = f.filename
		WHERE %s%s
		GROUP BY f.template, f.filename
		ORDER BY %s
		%s
	`, whereClause, tagAndClause, orderBySQL(opts.OrderBy), limitOffsetSQL(opts.Limit, opts.Offset))

	args := append([]any{}, whereArgs...)
	args = append(args, tagAndArgs...)

	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("index: query forms: %w", err)
	}
	defer rows.Close()

	out, err := scanFormRows(rows)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	facetsByKey, err := m.fetchFacetsFor(out)
	if err != nil {
		return nil, err
	}
	for i := range out {
		key := out[i].Template + "\x1f" + out[i].Filename
		if ff, ok := facetsByKey[key]; ok {
			out[i].Facets = ff
		}
	}
	return out, nil
}

// fetchFacetsFor pulls form_facets entries for the given set of FormRows
// in one round-trip and returns a lookup map keyed by template+US+filename.
// Caller stitches the per-row slice back onto each FormRow.
func (m *Manager) fetchFacetsFor(forms []FormRow) (map[string][]FormFacet, error) {
	if len(forms) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(forms)*2)
	placeholders := make([]string, 0, len(forms))
	for _, f := range forms {
		placeholders = append(placeholders, "(?,?)")
		args = append(args, f.Template, f.Filename)
	}
	q := fmt.Sprintf(`
		SELECT template, filename, facet_key, set_flag, selected
		FROM form_facets
		WHERE (template, filename) IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("index: query facets: %w", err)
	}
	defer rows.Close()

	out := map[string][]FormFacet{}
	for rows.Next() {
		var tpl, file, key string
		var setFlag int
		var sel sql.NullString
		if err := rows.Scan(&tpl, &file, &key, &setFlag, &sel); err != nil {
			return nil, fmt.Errorf("index: scan facet: %w", err)
		}
		k := tpl + "\x1f" + file
		out[k] = append(out[k], FormFacet{Key: key, Set: setFlag != 0, Selected: sel.String})
	}
	return out, rows.Err()
}

func orderBySQL(orderBy string) string {
	switch orderBy {
	case "updated_asc":
		return "f.updated ASC, f.filename ASC"
	case "title_asc":
		return "f.title ASC, f.filename ASC"
	case "title_desc":
		return "f.title DESC, f.filename ASC"
	case "filename_asc":
		return "f.filename ASC"
	case "filename_desc":
		return "f.filename DESC"
	default: // "updated_desc" or empty
		return "f.updated DESC, f.filename ASC"
	}
}

func limitOffsetSQL(limit, offset int) string {
	switch {
	case limit > 0 && offset > 0:
		return fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
	case limit > 0:
		return fmt.Sprintf("LIMIT %d", limit)
	case offset > 0:
		// SQLite requires LIMIT to use OFFSET; -1 = no limit.
		return fmt.Sprintf("LIMIT -1 OFFSET %d", offset)
	default:
		return ""
	}
}
