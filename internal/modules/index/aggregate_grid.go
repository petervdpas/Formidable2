package index

import (
	"database/sql"
	"fmt"
	"strings"
)

// AggDim is one grouping axis for AggregateRaw: a scalar field's value or
// a facet's selected option, optionally date-binned. DateWidth is the
// substr length applied to an ISO date (4 year, 7 month, 10 day); 0 means
// group by the raw text value.
type AggDim struct {
	Kind      string // "field" | "facet"
	Key       string
	DateWidth int
}

// StatRawRow is one form's contribution to an aggregation: the category
// label per dimension (aligned to AggregateRaw's dims) and the num_value
// of each requested numeric source (aligned to numKeys; invalid when the
// form has no value for that source).
type StatRawRow struct {
	Dims []string
	Nums []sql.NullFloat64
}

// AggregateRaw returns one row per form that has a value for every
// dimension, carrying each dimension's category label and each numeric
// source field's value. The statistical engine groups and reduces these
// in Go, so median/stddev/percentile compute uniformly with sum/avg/count.
//
// Scalar field and facet dimensions only; numeric sources are scalar
// fields (col IS NULL). Table-column sources are a later iteration - the
// engine rejects them before calling here. Dimensions INNER JOIN (a form
// missing a dimension value is excluded); numeric sources LEFT JOIN (a
// missing value is invalid, not a dropped row), so a count() measure still
// sees every form that has the grouping values.
func (m *Manager) AggregateRaw(template string, dims []AggDim, numKeys []string) ([]StatRawRow, error) {
	var sel, joins []string
	var args []any

	for i, d := range dims {
		alias := fmt.Sprintf("d%d", i)
		if d.Kind == "facet" {
			joins = append(joins, fmt.Sprintf(
				"JOIN form_facets %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.facet_key=? AND %[1]s.set_flag=1",
				alias))
			args = append(args, d.Key)
			sel = append(sel, fmt.Sprintf("COALESCE(%s.selected,'')", alias))
			continue
		}
		cond := fmt.Sprintf(
			"JOIN form_values %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.field_key=? AND %[1]s.col IS NULL AND %[1]s.text_value IS NOT NULL AND %[1]s.text_value<>''",
			alias)
		args = append(args, d.Key)
		if d.DateWidth > 0 {
			cond += fmt.Sprintf(" AND %s.value_type='date'", alias)
			joins = append(joins, cond)
			sel = append(sel, fmt.Sprintf("substr(%s.text_value,1,%d)", alias, d.DateWidth))
		} else {
			joins = append(joins, cond)
			sel = append(sel, fmt.Sprintf("%s.text_value", alias))
		}
	}

	for j, key := range numKeys {
		alias := fmt.Sprintf("n%d", j)
		joins = append(joins, fmt.Sprintf(
			"LEFT JOIN form_values %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.field_key=? AND %[1]s.col IS NULL AND %[1]s.num_value IS NOT NULL",
			alias))
		args = append(args, key)
		sel = append(sel, fmt.Sprintf("%s.num_value", alias))
	}

	if len(sel) == 0 {
		sel = append(sel, "1") // rank-0 count with no numeric source: one marker per form
	}

	q := "SELECT " + strings.Join(sel, ", ") + " FROM forms f " + strings.Join(joins, " ") + " WHERE f.template=?"
	args = append(args, template)

	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("index: aggregate raw %q: %w", template, err)
	}
	defer rows.Close()

	nd, nn := len(dims), len(numKeys)
	var out []StatRawRow
	for rows.Next() {
		dimVals := make([]sql.NullString, nd)
		numVals := make([]sql.NullFloat64, nn)
		dest := make([]any, 0, nd+nn+1)
		for i := range dimVals {
			dest = append(dest, &dimVals[i])
		}
		for j := range numVals {
			dest = append(dest, &numVals[j])
		}
		if nd+nn == 0 {
			var marker int
			dest = append(dest, &marker)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("index: scan aggregate raw: %w", err)
		}
		r := StatRawRow{Dims: make([]string, nd), Nums: numVals}
		for i, dv := range dimVals {
			r.Dims[i] = dv.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
