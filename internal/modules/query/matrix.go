package query

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func errUnknownSource(s Source) error {
	return fmt.Errorf("query: source not available in matrix: %s", sourceID(s))
}
func errGroupRange(i int) error { return fmt.Errorf("query: group column %d out of range", i) }
func errOrderRange(i int) error { return fmt.Errorf("query: order column %d out of range", i) }

// Matrix is the prepared in-memory grid: every cell a string, one column
// per queryable source, one row per (form x unnested table entry). Type
// is not stored; an operation reapplies it (parse to number / date) when
// it runs and tolerates cells that don't parse.
type Matrix struct {
	Cols []MatrixCol
	Rows []MatrixRow
	// FormCount is the number of source forms prepare read, regardless of
	// filtering or explosion: the "N of M" total.
	FormCount int
}

// MatrixCol describes one column. Hint is the declared type
// ("number"/"date"/"") used only to choose the default coercion, never to
// gate a value.
type MatrixCol struct {
	ID   string
	Hint string
}

// MatrixRow is one exploded row: Form is the source filename (so
// count_distinct counts forms, not rows); Cells aligns to Matrix.Cols.
// Origins records cartesian provenance, one Origin per participating
// table, so aggregates count each source row once despite the cartesian
// duplication.
type MatrixRow struct {
	Form    string
	Origins []Origin
	Cells   []string
}

// Origin links an exploded row back to one table it was fanned from. Hash
// makes identity the data (stable across reorders); Row is the positional
// index, kept so two identical-content rows stay distinct and aggregates
// don't undercount.
type Origin struct {
	Field string
	Row   int
	Hash  string
	Count int
}

// sourceID is the stable column key for a source, matching the frontend's
// srcId scheme (kind|key|col).
func sourceID(s Source) string {
	col := ""
	if s.Col != nil {
		col = strconv.Itoa(*s.Col)
	}
	return s.Kind + "|" + s.Key + "|" + col
}

// AggFuncs is the closed set of aggregate functions a Measure may use.
var AggFuncs = []string{"count", "count_distinct", "sum", "avg", "min", "max"}

func (m *Matrix) colIndex() map[string]int {
	idx := make(map[string]int, len(m.Cols))
	for i, c := range m.Cols {
		idx[c.ID] = i
	}
	return idx
}

// parseNum coerces a cell to a number; blank and non-numeric cells return
// ok=false.
func parseNum(s string) (float64, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// Execute runs a Spec against the prepared Matrix. Without GroupBy it is a
// filtered, projected, ordered row listing; with GroupBy it aggregates
// (group dimensions + Measures).
func (m *Matrix) Execute(spec Spec) (Result, error) {
	idx := m.colIndex()

	kept := m.filter(idx, spec.Filters)

	var res Result
	var err error
	if len(spec.GroupBy) > 0 {
		res, err = m.group(idx, kept, spec)
	} else {
		res, err = m.list(idx, kept, spec)
	}
	if err != nil {
		return Result{}, err
	}
	res.Anomalies = m.anomalies(referenced(spec))
	res.Total = m.FormCount
	return res, nil
}

// dateLayouts mirror the index/render date formats so the query treats a
// value as a valid date exactly when the rest of the app does.
var dateLayouts = []string{
	"2006-01-02",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

func parseDate(s string) bool {
	for _, l := range dateLayouts {
		if _, err := time.Parse(l, s); err == nil {
			return true
		}
	}
	return false
}

// referenced collects the source ids a spec touches, so anomaly scanning
// inspects only the typed columns the query uses.
func referenced(spec Spec) map[string]bool {
	used := map[string]bool{}
	for _, c := range spec.Columns {
		used[sourceID(c.Source)] = true
	}
	for _, f := range spec.Filters {
		used[sourceID(f.Source)] = true
	}
	for _, ms := range spec.Measures {
		if needsSource(ms.Func) {
			used[sourceID(ms.Source)] = true
		}
	}
	return used
}

// anomalies reports typed cells that won't coerce to their declared type.
// A blank cell is absence, not a violation, so it is skipped; a non-blank
// value that won't coerce means corrupt stored data.
func (m *Matrix) anomalies(used map[string]bool) []Anomaly {
	var out []Anomaly
	for ci, col := range m.Cols {
		if !used[col.ID] || (col.Hint != "number" && col.Hint != "date") {
			continue
		}
		for _, row := range m.Rows {
			if ci >= len(row.Cells) {
				continue
			}
			v := strings.TrimSpace(row.Cells[ci])
			if v == "" {
				continue
			}
			ok := parseDate(v)
			if col.Hint == "number" {
				_, ok = parseNum(v)
			}
			if !ok {
				out = append(out, Anomaly{Form: row.Form, Column: col.ID, Value: row.Cells[ci], Expected: col.Hint})
			}
		}
	}
	return out
}

// filter returns the row indices passing every filter. A filter on a
// source the matrix doesn't carry drops all rows, surfacing a bad spec
// rather than silently ignoring it.
func (m *Matrix) filter(idx map[string]int, filters []Filter) []int {
	out := make([]int, 0, len(m.Rows))
	for ri, row := range m.Rows {
		if passAll(row, idx, filters) {
			out = append(out, ri)
		}
	}
	return out
}

func passAll(row MatrixRow, idx map[string]int, filters []Filter) bool {
	for _, f := range filters {
		ci, ok := idx[sourceID(f.Source)]
		if !ok || ci >= len(row.Cells) {
			return false
		}
		if !passOne(row.Cells[ci], f) {
			return false
		}
	}
	return true
}

func passOne(cell string, f Filter) bool {
	switch f.Op {
	case "eq":
		return cell == f.Value
	case "ne":
		return cell != f.Value
	case "lt", "le", "gt", "ge":
		a, okA := parseNum(cell)
		b, okB := parseNum(f.Value)
		if !okA || !okB {
			return false
		}
		switch f.Op {
		case "lt":
			return a < b
		case "le":
			return a <= b
		case "gt":
			return a > b
		case "ge":
			return a >= b
		}
	}
	return false
}

// list projects the spec's columns, optionally de-duplicates the tuple,
// then orders and limits.
func (m *Matrix) list(idx map[string]int, kept []int, spec Spec) (Result, error) {
	cols := make([]int, len(spec.Columns))
	hints := make([]string, len(spec.Columns))
	for i, c := range spec.Columns {
		ci, ok := idx[sourceID(c.Source)]
		if !ok {
			return Result{}, errUnknownSource(c.Source)
		}
		cols[i] = ci
		hints[i] = m.Cols[ci].Hint
	}

	rows := make([][]Cell, 0, len(kept))
	seen := map[string]bool{}
	for _, ri := range kept {
		cells := make([]Cell, len(cols))
		for i, ci := range cols {
			cells[i] = cellOf(m.Rows[ri].Cells[ci], hints[i])
		}
		if spec.Distinct {
			key := tupleKey(cells)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		rows = append(rows, cells)
	}

	if err := orderRows(rows, spec.OrderBy, len(spec.Columns)); err != nil {
		return Result{}, err
	}
	rows = limitRows(rows, spec.Limit)

	res := Result{Columns: headers(spec.Columns), Rows: rows}
	res.Count = len(rows)
	return res, nil
}

// group aggregates the kept rows by their dimension tuple. When no
// Measures are given but Count is set, a single count column is emitted
// (back-compat with the old group/count shape).
func (m *Matrix) group(idx map[string]int, kept []int, spec Spec) (Result, error) {
	dimCols := make([]int, len(spec.GroupBy))
	dimHeaders := make([]string, len(spec.GroupBy))
	for i, gi := range spec.GroupBy {
		if gi < 0 || gi >= len(spec.Columns) {
			return Result{}, errGroupRange(gi)
		}
		ci, ok := idx[sourceID(spec.Columns[gi].Source)]
		if !ok {
			return Result{}, errUnknownSource(spec.Columns[gi].Source)
		}
		dimCols[i] = ci
		dimHeaders[i] = spec.Columns[gi].Header
	}

	measures := spec.Measures
	if len(measures) == 0 && spec.Count {
		measures = []Measure{{Func: "count", Header: countHeader(spec)}}
	}
	mCols := make([]int, len(measures))
	for i, ms := range measures {
		if needsSource(ms.Func) {
			ci, ok := idx[sourceID(ms.Source)]
			if !ok {
				return Result{}, errUnknownSource(ms.Source)
			}
			mCols[i] = ci
		} else {
			mCols[i] = -1
		}
	}

	type bucket struct {
		dims  []string
		rowIx []int
	}
	buckets := map[string]*bucket{}
	var order []string
	for _, ri := range kept {
		dv := make([]string, len(dimCols))
		for i, ci := range dimCols {
			dv[i] = m.Rows[ri].Cells[ci]
		}
		key := strings.Join(dv, "\x1f")
		b := buckets[key]
		if b == nil {
			b = &bucket{dims: dv}
			buckets[key] = b
			order = append(order, key)
		}
		b.rowIx = append(b.rowIx, ri)
	}
	sort.Strings(order)

	res := Result{Columns: append(append([]string{}, dimHeaders...), measureHeaders(measures)...)}
	for _, key := range order {
		b := buckets[key]
		cells := make([]Cell, 0, len(b.dims)+len(measures))
		for _, dv := range b.dims {
			cells = append(cells, Cell{Text: dv})
		}
		for i, ms := range measures {
			cells = append(cells, m.aggregate(ms.Func, mCols[i], ms.Source.Key, b.rowIx))
		}
		res.Rows = append(res.Rows, cells)
	}

	if err := orderRows(res.Rows, spec.OrderBy, len(res.Columns)); err != nil {
		return Result{}, err
	}
	res.Rows = limitRows(res.Rows, spec.Limit)
	res.Count = len(res.Rows)
	return res, nil
}

func (m *Matrix) aggregate(fn string, col int, srcField string, rowIx []int) Cell {
	switch fn {
	case "count":
		return numCell(float64(len(rowIx)))
	case "count_distinct":
		seen := map[string]bool{}
		for _, ri := range rowIx {
			seen[m.Rows[ri].Form] = true
		}
		return numCell(float64(len(seen)))
	case "sum", "avg", "min", "max":
		// Dedupe by the measure source's provenance so a cartesian fan
		// from another table can't inflate sum/avg: each (form, source
		// table row) contributes once. min/max are idempotent here.
		rowIx = m.dedupeBySource(rowIx, srcField)
		var sum, min, max float64
		n := 0
		for _, ri := range rowIx {
			v, ok := parseNum(m.Rows[ri].Cells[col])
			if !ok {
				continue
			}
			if n == 0 {
				min, max = v, v
			} else {
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
			sum += v
			n++
		}
		if n == 0 {
			return Cell{Text: ""}
		}
		switch fn {
		case "sum":
			return numCell(sum)
		case "avg":
			return numCell(sum / float64(n))
		case "min":
			return numCell(min)
		case "max":
			return numCell(max)
		}
	}
	return Cell{Text: ""}
}

func needsSource(fn string) bool {
	return fn == "sum" || fn == "avg" || fn == "min" || fn == "max"
}

// dedupeBySource keeps one row per (form, source-table row). A scalar
// source (no matching Origin) dedupes by form alone, so a scalar value
// fanned by a table is summed once per form, not once per table row.
func (m *Matrix) dedupeBySource(rowIx []int, srcField string) []int {
	seen := map[string]bool{}
	out := make([]int, 0, len(rowIx))
	for _, ri := range rowIx {
		id := m.Rows[ri].Form + "\x1f" + originRowKey(m.Rows[ri], srcField)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, ri)
	}
	return out
}

// originRowKey is the row's identity within a field: hash plus position.
// The position keeps duplicate-content rows distinct so a SUM counts both.
func originRowKey(row MatrixRow, field string) string {
	for _, o := range row.Origins {
		if o.Field == field {
			return o.Hash + ":" + strconv.Itoa(o.Row)
		}
	}
	return ""
}

// cellOf builds a typed Cell from a string; a number hint parses the
// value, with non-numeric cells staying text-only.
func cellOf(s, hint string) Cell {
	c := Cell{Text: s}
	if hint == "number" {
		if v, ok := parseNum(s); ok {
			c.Num = &v
		}
	}
	return c
}

func numCell(v float64) Cell {
	n := v
	return Cell{Text: strconv.FormatFloat(v, 'f', -1, 64), Num: &n}
}

func tupleKey(cells []Cell) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = c.Text
	}
	return strings.Join(parts, "\x1f")
}

// orderRows applies the multi-key stable sort. A numeric sort compares
// parsed numbers, with unparseable cells sorting last.
func orderRows(rows [][]Cell, orderBy []Sort, ncols int) error {
	for _, s := range orderBy {
		if s.Column < 0 || s.Column >= ncols {
			return errOrderRange(s.Column)
		}
	}
	if len(orderBy) == 0 {
		return nil
	}
	sort.SliceStable(rows, func(a, b int) bool {
		for _, s := range orderBy {
			ca, cb := rows[a][s.Column], rows[b][s.Column]
			cmp := compareCells(ca, cb, s.Numeric)
			if cmp == 0 {
				continue
			}
			if s.Desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
	return nil
}

func compareCells(a, b Cell, numeric bool) int {
	if numeric {
		na, okA := parseNum(a.Text)
		nb, okB := parseNum(b.Text)
		switch {
		case !okA && !okB:
			return 0
		case !okA:
			return 1
		case !okB:
			return -1
		case na < nb:
			return -1
		case na > nb:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(a.Text, b.Text)
}

func limitRows(rows [][]Cell, limit int) [][]Cell {
	if limit > 0 && len(rows) > limit {
		return rows[:limit]
	}
	return rows
}

func measureHeaders(measures []Measure) []string {
	out := make([]string, len(measures))
	for i, ms := range measures {
		out[i] = ms.Header
		if out[i] == "" {
			out[i] = ms.Func
		}
	}
	return out
}
