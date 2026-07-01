package index

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Manager owns the SQLite handle for one profile's index, exposing read-shaped methods; writes go through Reconcile.
type Manager struct {
	db *sql.DB
}

// NewManager opens (or creates) the index DB at path and migrates its schema.
func NewManager(path string) (*Manager, error) {
	db, err := openIndexDB(path)
	if err != nil {
		return nil, err
	}
	return &Manager{db: db}, nil
}

// DB exposes the raw handle. Low-level: tests use it for introspection and the
// same-package reconciler runs transactions through it. Domain callers should
// use the read/write methods below (Reconcile, ScanState, FormFilenames) so the
// index reads as a thing you ask to do work, not a SQL handle passed around.
func (m *Manager) DB() *sql.DB { return m.db }

// Reconcile applies a write batch in one transaction (bumping the revision).
// The domain write entry point: callers tell the Manager to reconcile rather
// than reaching for its DB.
func (m *Manager) Reconcile(b ReconcileBatch) error { return Reconcile(m.db, b) }

// ScanState reads the indexed (template, form) digest set rescan diffs against.
func (m *Manager) ScanState() (*indexState, error) { return scanIndexState(m.db) }

// FormFilenames returns the form basenames currently indexed under a template
// (from the index, not disk).
func (m *Manager) FormFilenames(templateFilename string) ([]string, error) {
	rows, err := m.db.Query(`SELECT filename FROM forms WHERE template = ?`, templateFilename)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Close releases the underlying handle.
func (m *Manager) Close() error { return m.db.Close() }

// Rev returns the monotonic revision counter (bumped per Reconcile batch), used as an ETag; fresh DB returns 0.
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
func (m *Manager) ListTemplates() ([]TemplateRow, error) {
	rows, err := m.db.Query(`
		SELECT filename, name, item_field, guid_field, tags_field,
		       has_markdown_template, enable_collection, presentation, mtime, size
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
		var hasMD, enableCol, presentation int
		if err := rows.Scan(
			&r.Filename, &r.Name, &r.ItemField, &r.GuidField, &r.TagsField,
			&hasMD, &enableCol, &presentation, &r.Mtime, &r.Size,
		); err != nil {
			return nil, fmt.Errorf("index: scan template: %w", err)
		}
		r.HasMarkdownTemplate = hasMD != 0
		r.EnableCollection = enableCol != 0
		r.Presentation = presentation != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListForms returns forms for one template, optionally filtered by
// tags (AND semantics) and shaped by QueryOpts.
func (m *Manager) ListForms(template string, opts QueryOpts) ([]FormRow, error) {
	return m.queryForms(formsByTemplateSQL{template: template}, opts)
}

// ListByTags returns forms across all templates owning every listed tag (AND).
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

// FormsWithValue returns filenames whose scalar value for fieldKey equals value; the datacore planner
// pushes this down so the tensor ingests only matching forms.
func (m *Manager) FormsWithValue(template, fieldKey, value string) ([]string, error) {
	rows, err := m.db.Query(`
		SELECT filename
		FROM form_values
		WHERE template = ? AND field_key = ? AND col IS NULL AND text_value = ?
	`, template, fieldKey, value)
	if err != nil {
		return nil, fmt.Errorf("index: forms with value %q.%q: %w", template, fieldKey, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, fmt.Errorf("index: scan filename: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// compareSQLOps maps a comparison operator to its SQL form. Fixed strings, so the
// op never reaches the query uninterpolated.
var compareSQLOps = map[string]string{"gt": ">", "ge": ">=", "lt": "<", "le": "<="}

// FormsWithValueOp returns filenames whose scalar value for fieldKey satisfies
// (op, value): eq/ne compare text_value; gt/ge/lt/le compare num_value against a
// parsed float (callers pass dates as epoch seconds, matching the value index).
// Only scalar entries (col IS NULL) are considered. Indexed fields only
// (use_in_statistics); a non-indexed field has no rows and yields none.
func (m *Manager) FormsWithValueOp(template, fieldKey, op, value string) ([]string, error) {
	var cond string
	var arg any
	switch op {
	case "eq":
		cond, arg = "text_value = ?", value
	case "ne":
		cond, arg = "text_value <> ?", value
	case "gt", "ge", "lt", "le":
		n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil, fmt.Errorf("index: forms with value %q.%q: numeric %q: %w", template, fieldKey, value, err)
		}
		cond, arg = "num_value "+compareSQLOps[op]+" ?", n
	default:
		return nil, fmt.Errorf("index: forms with value %q.%q: invalid op %q", template, fieldKey, op)
	}
	rows, err := m.db.Query(`
		SELECT filename
		FROM form_values
		WHERE template = ? AND field_key = ? AND col IS NULL AND `+cond,
		template, fieldKey, arg)
	if err != nil {
		return nil, fmt.Errorf("index: forms with value op %q.%q: %w", template, fieldKey, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, fmt.Errorf("index: scan filename: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// MaxValue returns the greatest scalar num_value for fieldKey across a
// template's records (col IS NULL = scalar only). ok=false when the field has
// no numeric rows yet (empty collection, or values that never parsed). Backs
// sequence auto-assign: the next record starts at MaxValue + step.
func (m *Manager) MaxValue(template, fieldKey string) (float64, bool, error) {
	var max sql.NullFloat64
	err := m.db.QueryRow(`
		SELECT MAX(num_value)
		FROM form_values
		WHERE template = ? AND field_key = ? AND col IS NULL
	`, template, fieldKey).Scan(&max)
	if err != nil {
		return 0, false, fmt.Errorf("index: max value %q.%q: %w", template, fieldKey, err)
	}
	if !max.Valid {
		return 0, false, nil
	}
	return max.Float64, true, nil
}

// formsQuerySpec is one WHERE-shape the read API needs; QueryOpts adds tag/order/limit on top.
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

// queryForms builds the forms SELECT (tags re-attached via GROUP_CONCAT), applies the spec WHERE, the
// optional tags AND-filter, order, and limit/offset. The GROUP_CONCAT separator is US (\x1f), which tags never contain.
func (m *Manager) queryForms(spec formsQuerySpec, opts QueryOpts) ([]FormRow, error) {
	whereClause, whereArgs := spec.where()

	var tagAndClause string
	tagAndArgs := []any{}
	if len(opts.Tags) > 0 {
		// COUNT(DISTINCT tag)=N selects forms with every requested tag (PK prevents duplicates).
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

// fetchFacetsFor pulls form_facets for the given rows in one round-trip, keyed by template+US+filename.
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
		// SQLite needs LIMIT for OFFSET; -1 = no limit.
		return fmt.Sprintf("LIMIT -1 OFFSET %d", offset)
	default:
		return ""
	}
}
