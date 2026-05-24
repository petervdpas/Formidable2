package index

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// AggDim is one grouping axis for AggregateRaw: a scalar field's value, a
// table column's value (Col set), or a facet's selected option, optionally
// date-binned. Col is the positional form_values.col index (nil = scalar
// field; ignored for facets). DateWidth is the substr length applied to an
// ISO date (4 year, 7 month, 10 day); 0 = raw text value.
type AggDim struct {
	Kind      string // "field" | "facet"
	Key       string
	Col       *int
	DateWidth int
}

// AggNum is a numeric source for a reducing measure: a scalar field
// (Col nil) or a table column (Col set).
type AggNum struct {
	Key string
	Col *int
}

// AggFilter scopes the aggregation to rows where the source satisfies the
// comparison. Op is "eq"/"ne" (text compare) or "lt"/"le"/"gt"/"ge"
// (numeric compare). Kind/Key/Col mirror AggDim; a table-column filter
// (Col set) fans to the matching cells, which is the point of e.g.
// "where stored-procedures.procedure eq X".
type AggFilter struct {
	Kind  string // "field" | "facet"
	Key   string
	Col   *int
	Op    string
	Value string
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
// Scalar field, table-column and facet dimensions; numeric sources are
// scalar fields or table columns. A column join (Col set) fans to one row
// per matching cell - the caller (stat engine) limits a statistic to a
// single such one-to-many source to avoid a cartesian over-count.
// Dimensions INNER JOIN (a form missing a dimension value is excluded);
// numeric sources LEFT JOIN (a missing value is invalid, not a dropped
// row), so a count() measure still sees every matching row.
func (m *Manager) AggregateRaw(template string, dims []AggDim, nums []AggNum, filters []AggFilter) ([]StatRawRow, error) {
	var sel, joins []string
	var args []any

	// colPred returns the SQL for matching a form_values column plus the
	// extra arg (if any); a nil col matches the scalar (col IS NULL) row.
	colPred := func(alias string, col *int) (string, []any) {
		if col == nil {
			return alias + ".col IS NULL", nil
		}
		return alias + ".col = ?", []any{*col}
	}

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
		pred, pArgs := colPred(alias, d.Col)
		cond := fmt.Sprintf(
			"JOIN form_values %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.field_key=? AND %[2]s AND %[1]s.text_value IS NOT NULL AND %[1]s.text_value<>''",
			alias, pred)
		args = append(args, d.Key)
		args = append(args, pArgs...)
		if d.DateWidth > 0 {
			cond += fmt.Sprintf(" AND %s.value_type='date'", alias)
			joins = append(joins, cond)
			sel = append(sel, fmt.Sprintf("substr(%s.text_value,1,%d)", alias, d.DateWidth))
		} else {
			joins = append(joins, cond)
			sel = append(sel, fmt.Sprintf("%s.text_value", alias))
		}
	}

	for j, n := range nums {
		alias := fmt.Sprintf("n%d", j)
		pred, pArgs := colPred(alias, n.Col)
		joins = append(joins, fmt.Sprintf(
			"LEFT JOIN form_values %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.field_key=? AND %[2]s AND %[1]s.num_value IS NOT NULL",
			alias, pred))
		args = append(args, n.Key)
		args = append(args, pArgs...)
		sel = append(sel, fmt.Sprintf("%s.num_value", alias))
	}

	cmpSym := map[string]string{"lt": "<", "le": "<=", "gt": ">", "ge": ">="}
	for k, fl := range filters {
		alias := fmt.Sprintf("w%d", k)
		if fl.Kind == "facet" {
			op := "="
			if fl.Op == "ne" {
				op = "<>"
			}
			joins = append(joins, fmt.Sprintf(
				"JOIN form_facets %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.facet_key=? AND %[1]s.set_flag=1 AND COALESCE(%[1]s.selected,'') %[2]s ?",
				alias, op))
			args = append(args, fl.Key, fl.Value)
			continue
		}
		pred, pArgs := colPred(alias, fl.Col)
		var cond string
		var arg any
		switch fl.Op {
		case "eq":
			cond, arg = alias+".text_value = ?", fl.Value
		case "ne":
			cond, arg = alias+".text_value <> ?", fl.Value
		case "lt", "le", "gt", "ge":
			n, err := strconv.ParseFloat(fl.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("index: filter %s needs a number, got %q", fl.Op, fl.Value)
			}
			cond, arg = fmt.Sprintf("%s.num_value %s ?", alias, cmpSym[fl.Op]), n
		default:
			return nil, fmt.Errorf("index: unknown filter op %q", fl.Op)
		}
		joins = append(joins, fmt.Sprintf(
			"JOIN form_values %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.field_key=? AND %[2]s AND %[3]s",
			alias, pred, cond))
		args = append(args, fl.Key)
		args = append(args, pArgs...)
		args = append(args, arg)
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

	nd, nn := len(dims), len(nums)
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
