package query

import (
	"fmt"
	"strconv"
	"strings"
)

// Explain renders a read-only, SQL-shaped preview of a spec. It is the
// authoritative rendering (the same module that runs the query produces
// it), resolving source labels from the template so the text matches what
// the engine actually does. It is a preview only - the engine runs the
// typed spec, not this string.
func (m *Manager) Explain(spec Spec) (string, error) {
	if strings.TrimSpace(spec.Template) == "" {
		return "", fmt.Errorf("query: template required")
	}
	tpl, err := m.loader.Template(spec.Template)
	if err != nil {
		return "", err
	}
	name := spec.Template
	label := map[string]string{}
	numeric := map[string]bool{}
	if tpl != nil {
		if tpl.Name != "" {
			name = tpl.Name
		}
		for _, s := range deriveSources(tpl) {
			label[s.ID] = s.Label
			numeric[s.ID] = s.Numeric
		}
	}
	return renderSQL(spec, name, label, numeric), nil
}

func renderSQL(spec Spec, name string, label map[string]string, numeric map[string]bool) string {
	lab := func(s Source) string {
		if l := label[sourceID(s)]; l != "" {
			return l
		}
		return s.Key
	}
	colHeader := func(c Column) string {
		if c.Header != "" {
			return c.Header
		}
		return lab(c.Source)
	}
	grouped := len(spec.GroupBy) > 0

	var sel, outLabels []string
	if grouped {
		for _, gi := range spec.GroupBy {
			if gi < 0 || gi >= len(spec.Columns) {
				continue
			}
			h := colHeader(spec.Columns[gi])
			sel = append(sel, ident(h))
			outLabels = append(outLabels, h)
		}
		measures := spec.Measures
		if len(measures) == 0 && spec.Count {
			measures = []Measure{{Func: "count", Header: countHeader(spec)}}
		}
		for _, ms := range measures {
			sel = append(sel, measureSQL(ms, lab))
			outLabels = append(outLabels, measureHeader(ms))
		}
	} else {
		for _, c := range spec.Columns {
			h := colHeader(c)
			sel = append(sel, ident(h))
			outLabels = append(outLabels, h)
		}
	}
	if len(sel) == 0 {
		sel = []string{"*"}
	}
	distinct := ""
	if !grouped && spec.Distinct {
		distinct = "DISTINCT "
	}

	lines := []string{
		"SELECT " + distinct + strings.Join(sel, ", "),
		"FROM " + ident(name),
	}

	var where []string
	for _, f := range spec.Filters {
		v := f.Value
		if numeric[sourceID(f.Source)] && isNumber(v) {
			// leave numeric literals unquoted
		} else {
			v = "'" + strings.ReplaceAll(v, "'", "''") + "'"
		}
		where = append(where, ident(lab(f.Source))+" "+sqlOp(f.Op)+" "+v)
	}
	if len(where) > 0 {
		lines = append(lines, "WHERE "+strings.Join(where, "\n  AND "))
	}

	if grouped {
		var dims []string
		for _, gi := range spec.GroupBy {
			if gi < 0 || gi >= len(spec.Columns) {
				continue
			}
			dims = append(dims, ident(colHeader(spec.Columns[gi])))
		}
		if len(dims) > 0 {
			lines = append(lines, "GROUP BY "+strings.Join(dims, ", "))
		}
	}

	var ord []string
	for _, s := range spec.OrderBy {
		if s.Column < 0 || s.Column >= len(outLabels) {
			continue
		}
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		ord = append(ord, ident(outLabels[s.Column])+" "+dir)
	}
	if len(ord) > 0 {
		lines = append(lines, "ORDER BY "+strings.Join(ord, ", "))
	}

	if spec.Limit > 0 {
		lines = append(lines, "LIMIT "+strconv.Itoa(spec.Limit))
	}

	return strings.Join(lines, "\n") + ";"
}

func measureSQL(ms Measure, lab func(Source) string) string {
	var inner string
	switch ms.Func {
	case "count":
		inner = "COUNT(*)"
	case "count_distinct":
		inner = "COUNT(DISTINCT record)"
	case "sum", "avg", "min", "max":
		inner = strings.ToUpper(ms.Func) + "(" + ident(lab(ms.Source)) + ")"
	default:
		inner = ms.Func
	}
	return inner + " AS " + ident(measureHeader(ms))
}

func measureHeader(ms Measure) string {
	if ms.Header != "" {
		return ms.Header
	}
	return ms.Func
}

func ident(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func sqlOp(op string) string {
	switch op {
	case "eq":
		return "="
	case "ne":
		return "<>"
	case "lt":
		return "<"
	case "le":
		return "<="
	case "gt":
		return ">"
	case "ge":
		return ">="
	}
	return op
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil
}
