package index

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// ProjectCol is one column in a row-listing projection: a scalar field
// value, a table column value (Col set), or a facet's selected option.
// Unlike AggDim there is no date-bin - projection returns raw values.
type ProjectCol struct {
	Kind string // "field" | "facet"
	Key  string
	Col  *int
}

// ProjectSort orders the result by a projected column (Index into Cols).
// Numeric sorts on num_value rather than text_value, so a number column
// orders 2 < 10 instead of "10" < "2".
type ProjectSort struct {
	Index   int
	Desc    bool
	Numeric bool
}

// ProjectSpec is a row-listing query over form_values: which columns to
// project, an optional WHERE (reusing AggFilter), DISTINCT over the
// projected tuple, ORDER BY projected columns, and a LIMIT. A
// multi-valued column (a table column, or a list/tags field stored
// one-row-per-entry) fans the output one row per entry, which is the
// flatten/explode behaviour. Projecting two independent multi-valued
// columns cross-joins them; the query layer guards against that.
type ProjectSpec struct {
	Cols     []ProjectCol
	Filters  []AggFilter
	Distinct bool
	OrderBy  []ProjectSort
	Limit    int
}

// ProjectCell is one projected value: the display text plus the parsed
// number where the cell carried one (num-invalid for text or for a
// LEFT-JOIN miss).
type ProjectCell struct {
	Text string
	Num  sql.NullFloat64
}

// ProjectRow is one projected row: the source form filename (empty in
// DISTINCT mode, where rows are value tuples, not forms) and the cell
// per projected column, aligned to ProjectSpec.Cols.
type ProjectRow struct {
	Form  string
	Cells []ProjectCell
}

// ProjectRows lists per-form rows over form_values. Projected columns
// LEFT JOIN so a form missing a value still appears (an empty,
// num-invalid cell) rather than being dropped the way AggregateRaw's
// grouping INNER JOIN would. Filters scope the result with the same
// semantics AggregateRaw uses. The query module builds this for its
// row-listing and flatten-and-distinct modes; the grouping/count modes
// reuse AggregateRaw directly.
func (m *Manager) ProjectRows(template string, spec ProjectSpec) ([]ProjectRow, error) {
	var sel, joins []string
	var args []any

	// A filter targeting a column we also project is folded into that
	// column's join (same alias) so the projected cell IS the filtered
	// cell. Without this, a filter on a multi-valued (table/list) column
	// matches at the form level while the projection fans every cell, so
	// "Application = X" wrongly returns all of a matching form's
	// applications. Folding turns that column's LEFT JOIN into an INNER
	// JOIN carrying the predicate, so only matching cells survive the fan.
	// Filters that match no projected column still scope at the form level.
	used := make([]bool, len(spec.Filters))

	for i, c := range spec.Cols {
		alias := fmt.Sprintf("c%d", i)
		var conds []string
		var condArgs []any
		for k := range spec.Filters {
			if used[k] || !sameSource(spec.Filters[k], c) {
				continue
			}
			cond, cargs, err := colFilterCond(alias, spec.Filters[k])
			if err != nil {
				return nil, err
			}
			conds = append(conds, cond)
			condArgs = append(condArgs, cargs...)
			used[k] = true
		}
		// A folded filter restricts rows, so the join must be INNER; an
		// unfiltered projected column stays LEFT so a form missing it
		// still appears.
		join := "LEFT JOIN"
		if len(conds) > 0 {
			join = "JOIN"
		}

		if c.Kind == "facet" {
			on := fmt.Sprintf("%[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.facet_key=? AND %[1]s.set_flag=1", alias)
			args = append(args, c.Key)
			if len(conds) > 0 {
				on += " AND " + strings.Join(conds, " AND ")
				args = append(args, condArgs...)
			}
			joins = append(joins, fmt.Sprintf("%s form_facets %s ON %s", join, alias, on))
			// NULL keeps the (text, num) pair shape uniform across columns;
			// a facet has no numeric value.
			sel = append(sel, fmt.Sprintf("COALESCE(%s.selected,'')", alias), "NULL")
			continue
		}
		pred, pArgs := colPred(alias, c.Col)
		on := fmt.Sprintf("%[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.field_key=? AND %[2]s", alias, pred)
		args = append(args, c.Key)
		args = append(args, pArgs...)
		if len(conds) > 0 {
			on += " AND " + strings.Join(conds, " AND ")
			args = append(args, condArgs...)
		}
		joins = append(joins, fmt.Sprintf("%s form_values %s ON %s", join, alias, on))
		sel = append(sel, fmt.Sprintf("%s.text_value", alias), fmt.Sprintf("%s.num_value", alias))
	}

	remaining := make([]AggFilter, 0, len(spec.Filters))
	for k, fl := range spec.Filters {
		if !used[k] {
			remaining = append(remaining, fl)
		}
	}
	fJoins, fArgs, err := filterJoins(remaining)
	if err != nil {
		return nil, err
	}
	joins = append(joins, fJoins...)

	var q strings.Builder
	q.WriteString("SELECT ")
	if spec.Distinct {
		q.WriteString("DISTINCT ")
		q.WriteString(strings.Join(sel, ", "))
	} else {
		// f.filename leads so each row keeps its source form (and the
		// no-column case still selects a real column).
		q.WriteString(strings.Join(append([]string{"f.filename"}, sel...), ", "))
	}
	q.WriteString(" FROM forms f ")
	q.WriteString(strings.Join(joins, " "))
	q.WriteString(" WHERE f.template=?")

	allArgs := append(args, fArgs...)
	allArgs = append(allArgs, template)

	if ord := orderClause(spec.Cols, spec.OrderBy); ord != "" {
		q.WriteString(ord)
	}
	if spec.Limit > 0 {
		q.WriteString(" LIMIT ?")
		allArgs = append(allArgs, spec.Limit)
	}

	rows, err := m.db.Query(q.String(), allArgs...)
	if err != nil {
		return nil, fmt.Errorf("index: project rows %q: %w", template, err)
	}
	defer rows.Close()

	nc := len(spec.Cols)
	var out []ProjectRow
	for rows.Next() {
		var form string
		texts := make([]sql.NullString, nc)
		nums := make([]sql.NullFloat64, nc)
		dest := make([]any, 0, 1+2*nc)
		if !spec.Distinct {
			dest = append(dest, &form)
		}
		for i := range texts {
			dest = append(dest, &texts[i], &nums[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("index: scan project row: %w", err)
		}
		r := ProjectRow{Form: form, Cells: make([]ProjectCell, nc)}
		for i := range texts {
			r.Cells[i] = ProjectCell{Text: texts[i].String, Num: nums[i]}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// orderClause builds the ORDER BY for a projection. Out-of-range sort
// indices are skipped rather than erroring, so a stale UI selection
// degrades to "no sort" instead of failing the whole query.
func orderClause(cols []ProjectCol, sorts []ProjectSort) string {
	var parts []string
	for _, s := range sorts {
		if s.Index < 0 || s.Index >= len(cols) {
			continue
		}
		alias := fmt.Sprintf("c%d", s.Index)
		var expr string
		switch {
		case cols[s.Index].Kind == "facet":
			expr = fmt.Sprintf("COALESCE(%s.selected,'')", alias)
		case s.Numeric:
			expr = alias + ".num_value"
		default:
			expr = alias + ".text_value"
		}
		if s.Desc {
			expr += " DESC"
		}
		parts = append(parts, expr)
	}
	if len(parts) == 0 {
		return ""
	}
	return " ORDER BY " + strings.Join(parts, ", ")
}

// FilterOps lists the comparison operators AggFilter accepts, in display
// order: eq/ne compare text, lt/le/gt/ge compare numbers. The single
// published source for a query UI's operator picker; filterJoins is the
// validator and a test guards the two against drift.
var FilterOps = []string{"eq", "ne", "lt", "le", "gt", "ge"}

// colPred returns the SQL predicate matching a form_values column plus
// any extra arg; a nil col matches the scalar (col IS NULL) row. Shared
// by AggregateRaw and ProjectRows.
func colPred(alias string, col *int) (string, []any) {
	if col == nil {
		return alias + ".col IS NULL", nil
	}
	return alias + ".col = ?", []any{*col}
}

var cmpSym = map[string]string{"lt": "<", "le": "<=", "gt": ">", "ge": ">="}

// sameSource reports whether a filter targets the same indexed value as a
// projected column (kind + key + table column index), so the filter can be
// folded into that column's join.
func sameSource(fl AggFilter, c ProjectCol) bool {
	if fl.Kind != c.Kind || fl.Key != c.Key {
		return false
	}
	if (fl.Col == nil) != (c.Col == nil) {
		return false
	}
	return fl.Col == nil || *fl.Col == *c.Col
}

// colFilterCond builds the boolean predicate for one filter against a
// joined alias (no table join - just the comparison on that alias's
// columns), plus its args. eq/ne compare text_value; lt/le/gt/ge compare
// num_value; a facet compares its selected option (eq/ne, "=" default to
// preserve the historical lenient behavior).
func colFilterCond(alias string, fl AggFilter) (string, []any, error) {
	if fl.Kind == "facet" {
		op := "="
		if fl.Op == "ne" {
			op = "<>"
		}
		return fmt.Sprintf("COALESCE(%s.selected,'') %s ?", alias, op), []any{fl.Value}, nil
	}
	switch fl.Op {
	case "eq":
		return alias + ".text_value = ?", []any{fl.Value}, nil
	case "ne":
		return alias + ".text_value <> ?", []any{fl.Value}, nil
	case "lt", "le", "gt", "ge":
		n, err := strconv.ParseFloat(fl.Value, 64)
		if err != nil {
			return "", nil, fmt.Errorf("index: filter %s needs a number, got %q", fl.Op, fl.Value)
		}
		return fmt.Sprintf("%s.num_value %s ?", alias, cmpSym[fl.Op]), []any{n}, nil
	default:
		return "", nil, fmt.Errorf("index: unknown filter op %q", fl.Op)
	}
}

// filterJoins builds the JOIN clauses (and their args, in order) that
// scope a query to rows satisfying every AggFilter. Shared by
// AggregateRaw and ProjectRows so the WHERE semantics stay identical.
// Used for filters NOT folded into a projected column - they restrict at
// the form level (the form has at least one matching cell).
func filterJoins(filters []AggFilter) ([]string, []any, error) {
	var joins []string
	var args []any
	for k, fl := range filters {
		alias := fmt.Sprintf("w%d", k)
		cond, cargs, err := colFilterCond(alias, fl)
		if err != nil {
			return nil, nil, err
		}
		if fl.Kind == "facet" {
			joins = append(joins, fmt.Sprintf(
				"JOIN form_facets %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.facet_key=? AND %[1]s.set_flag=1 AND %[2]s",
				alias, cond))
			args = append(args, fl.Key)
			args = append(args, cargs...)
			continue
		}
		pred, pArgs := colPred(alias, fl.Col)
		joins = append(joins, fmt.Sprintf(
			"JOIN form_values %[1]s ON %[1]s.template=f.template AND %[1]s.filename=f.filename AND %[1]s.field_key=? AND %[2]s AND %[3]s",
			alias, pred, cond))
		args = append(args, fl.Key)
		args = append(args, pArgs...)
		args = append(args, cargs...)
	}
	return joins, args, nil
}
