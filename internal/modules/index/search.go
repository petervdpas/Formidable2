package index

import (
	"database/sql"
	"fmt"
	"strings"
)

// SearchForms runs a full-text query over one template's forms, ranked by
// FTS5 relevance. buildMatchQuery sanitizes the free-text query so raw FTS5
// operators never reach the engine; an empty/whitespace query returns no
// rows (not an error). opts.OrderBy is ignored: relevance ranking is the
// point of a search.
func (m *Manager) SearchForms(template, query string, opts QueryOpts) ([]FormRow, error) {
	match := buildMatchQuery(query)
	if match == "" {
		return nil, nil
	}

	q := fmt.Sprintf(`
		SELECT f.template, f.filename, f.id, f.title, f.fm_title,
		       f.created, f.created_name, f.created_email,
		       f.updated, f.updated_name, f.updated_email,
		       f.expression_items, f.mtime, f.size,
		       COALESCE(GROUP_CONCAT(t.tag, char(31)), '') AS tags
		FROM form_fts
		JOIN form_search s ON s.rowid = form_fts.rowid
		JOIN forms f ON f.template = s.template AND f.filename = s.filename
		LEFT JOIN form_tags t ON t.template = f.template AND t.filename = f.filename
		WHERE form_fts MATCH ? AND s.template = ?
		GROUP BY f.template, f.filename
		ORDER BY form_fts.rank
		%s
	`, limitOffsetSQL(opts.Limit, opts.Offset))

	rows, err := m.db.Query(q, match, template)
	if err != nil {
		return nil, fmt.Errorf("index: search forms: %w", err)
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

// scanFormRows scans the shared SELECT column list into FormRows, splitting
// the US-joined tags back into a slice. Facet stitching is the caller's job.
func scanFormRows(rows *sql.Rows) ([]FormRow, error) {
	var (
		out   []FormRow
		ccn   sql.NullString
		cce   sql.NullString
		ucn   sql.NullString
		uce   sql.NullString
		expI  sql.NullString
		fmTit sql.NullString
		idCol sql.NullString
		title sql.NullString
		creAt sql.NullString
		updAt sql.NullString
	)
	for rows.Next() {
		var r FormRow
		var tagsJoined string
		if err := rows.Scan(
			&r.Template, &r.Filename, &idCol, &title, &fmTit,
			&creAt, &ccn, &cce,
			&updAt, &ucn, &uce,
			&expI, &r.Mtime, &r.Size,
			&tagsJoined,
		); err != nil {
			return nil, fmt.Errorf("index: scan form: %w", err)
		}
		r.ID = idCol.String
		r.Title = title.String
		r.FmTitle = fmTit.String
		r.Created = creAt.String
		r.CreatedName = ccn.String
		r.CreatedEmail = cce.String
		r.Updated = updAt.String
		r.UpdatedName = ucn.String
		r.UpdatedEmail = uce.String
		r.ExpressionItems = expI.String
		if tagsJoined != "" {
			r.Tags = strings.Split(tagsJoined, "\x1f")
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// buildMatchQuery turns free user text into a safe FTS5 MATCH string: each
// run of token characters becomes a quoted prefix term ("word"*), joined by
// spaces (FTS5 implicit AND). Quoting neutralises every FTS5 operator so
// arbitrary input can never produce a syntax error. Empty input returns "".
func buildMatchQuery(raw string) string {
	var terms []string
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool { return !isTokenRune(r) }) {
		terms = append(terms, `"`+field+`"*`)
	}
	return strings.Join(terms, " ")
}

// isTokenRune reports whether r is part of a search token. Keeping only
// letters and digits also strips FTS5 operator characters before quoting.
func isTokenRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r > 127: // keep non-ASCII letters (accented, CJK, ...)
		return true
	default:
		return false
	}
}
