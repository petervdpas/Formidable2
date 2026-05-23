package index

import (
	"database/sql"
	"fmt"
)

// Bucket is one (label, count) pair from a distribution or date series.
type Bucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// CrossCell is one (a, b, count) triple from a cross-tab between two
// facet keys.
type CrossCell struct {
	A     string `json:"a"`
	B     string `json:"b"`
	Count int    `json:"count"`
}

// TotalForms is the denominator for percentage stats: how many forms
// exist for a template.
func (m *Manager) TotalForms(template string) (int, error) {
	var n int
	err := m.db.QueryRow(`SELECT COUNT(*) FROM forms WHERE template = ?`, template).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("index: total forms %q: %w", template, err)
	}
	return n, nil
}

// colPredicate builds the "col IS NULL" (scalar field) or "col = ?"
// (table column) clause plus its args, so distribution/numeric queries
// share one column-matching rule. A nil col means the scalar row.
func colPredicate(col *int) (string, []any) {
	if col == nil {
		return "col IS NULL", nil
	}
	return "col = ?", []any{*col}
}

// ValueDistribution counts forms grouped by text_value for one field
// (col nil) or one table column (col set). Empty text is skipped so a
// blank cell doesn't show as a category. Ordered by label for stable
// output.
func (m *Manager) ValueDistribution(template, fieldKey string, col *int) ([]Bucket, error) {
	pred, predArgs := colPredicate(col)
	args := append([]any{template, fieldKey}, predArgs...)
	rows, err := m.db.Query(`
		SELECT text_value, COUNT(*)
		FROM form_values
		WHERE template = ? AND field_key = ? AND `+pred+`
		  AND text_value IS NOT NULL AND text_value <> ''
		GROUP BY text_value
		ORDER BY text_value
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("index: value distribution %q.%q: %w", template, fieldKey, err)
	}
	return scanBuckets(rows)
}

// NumericValues returns the raw num_value column for one field / table
// column, NULLs excluded. The stat layer runs median / stddev /
// percentile over these (SQLite has no built-ins for those).
func (m *Manager) NumericValues(template, fieldKey string, col *int) ([]float64, error) {
	pred, predArgs := colPredicate(col)
	args := append([]any{template, fieldKey}, predArgs...)
	rows, err := m.db.Query(`
		SELECT num_value
		FROM form_values
		WHERE template = ? AND field_key = ? AND `+pred+`
		  AND num_value IS NOT NULL
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("index: numeric values %q.%q: %w", template, fieldKey, err)
	}
	defer rows.Close()
	var out []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("index: scan numeric: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// FacetDistribution counts set forms grouped by the selected option of
// one facet key. Hits idx_form_facets_lookup. Forms with the facet set
// but no option chosen group under the empty label, which the caller
// can present as "(unset)".
func (m *Manager) FacetDistribution(template, facetKey string) ([]Bucket, error) {
	rows, err := m.db.Query(`
		SELECT COALESCE(selected, ''), COUNT(*)
		FROM form_facets
		WHERE template = ? AND facet_key = ? AND set_flag = 1
		GROUP BY selected
		ORDER BY selected
	`, template, facetKey)
	if err != nil {
		return nil, fmt.Errorf("index: facet distribution %q.%q: %w", template, facetKey, err)
	}
	return scanBuckets(rows)
}

// FacetCross is the pair-combination matrix between two facet keys: for
// each form that has both facets set, one (selectedA, selectedB) cell.
// Self-join on (template, filename) over the set rows.
func (m *Manager) FacetCross(template, keyA, keyB string) ([]CrossCell, error) {
	rows, err := m.db.Query(`
		SELECT COALESCE(a.selected, ''), COALESCE(b.selected, ''), COUNT(*)
		FROM form_facets a
		JOIN form_facets b
		  ON a.template = b.template AND a.filename = b.filename
		WHERE a.template = ? AND a.facet_key = ? AND a.set_flag = 1
		  AND b.facet_key = ? AND b.set_flag = 1
		GROUP BY a.selected, b.selected
		ORDER BY a.selected, b.selected
	`, template, keyA, keyB)
	if err != nil {
		return nil, fmt.Errorf("index: facet cross %q.(%q,%q): %w", template, keyA, keyB, err)
	}
	defer rows.Close()
	var out []CrossCell
	for rows.Next() {
		var c CrossCell
		if err := rows.Scan(&c.A, &c.B, &c.Count); err != nil {
			return nil, fmt.Errorf("index: scan cross cell: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DateSeries buckets a date field / table column by period and counts
// forms per bucket. period is "year" (YYYY), "month" (YYYY-MM) or "day"
// (YYYY-MM-DD); anything else defaults to month. text_value already
// holds the ISO date, so a substring prefix is the bucket key. Ordered
// chronologically (ISO sorts lexically).
func (m *Manager) DateSeries(template, fieldKey string, col *int, period string) ([]Bucket, error) {
	width := 7
	switch period {
	case "year":
		width = 4
	case "day":
		width = 10
	}
	pred, predArgs := colPredicate(col)
	args := append([]any{width, template, fieldKey}, predArgs...)
	rows, err := m.db.Query(`
		SELECT substr(text_value, 1, ?) AS bucket, COUNT(*)
		FROM form_values
		WHERE template = ? AND field_key = ? AND `+pred+`
		  AND value_type = 'date' AND text_value IS NOT NULL AND text_value <> ''
		GROUP BY bucket
		ORDER BY bucket
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("index: date series %q.%q: %w", template, fieldKey, err)
	}
	return scanBuckets(rows)
}

func scanBuckets(rows *sql.Rows) ([]Bucket, error) {
	defer rows.Close()
	var out []Bucket
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.Label, &b.Count); err != nil {
			return nil, fmt.Errorf("index: scan bucket: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
