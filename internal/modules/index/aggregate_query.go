package index

import (
	"fmt"
	"strconv"
)

// This file holds the shared WHERE building blocks for queries over
// form_values: the operator list, the column predicate, and the filter
// join builder. AggregateRaw (aggregate_grid.go) uses them so its filter
// semantics have a single definition.

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
