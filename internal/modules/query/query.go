package query

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// Index is the narrow slice of index.Manager the query module needs.
// *index.Manager satisfies it; tests pass a fake. Mirrors stat.Index -
// query is a peer consumer of the same datacore.
type Index interface {
	ProjectRows(template string, spec index.ProjectSpec) ([]index.ProjectRow, error)
	AggregateRaw(template string, dims []index.AggDim, nums []index.AggNum, filters []index.AggFilter) ([]index.StatRawRow, error)
	TotalForms(template string) (int, error)
}

// Manager translates a Spec into datacore calls and shapes the result.
// Stateless beyond its index dependency.
type Manager struct {
	idx Index
}

func NewManager(idx Index) *Manager { return &Manager{idx: idx} }

const defaultCountHeader = "count"

// Run executes a query. With no GroupBy it is a row listing
// (index.ProjectRows); with GroupBy it is a group/count aggregation
// (index.AggregateRaw).
func (m *Manager) Run(spec Spec) (Result, error) {
	if strings.TrimSpace(spec.Template) == "" {
		return Result{}, fmt.Errorf("query: template required")
	}
	if len(spec.GroupBy) > 0 {
		return m.runGroup(spec)
	}
	return m.runList(spec)
}

// runList projects rows via ProjectRows. Distinct, OrderBy and Limit pass
// straight through to the datacore.
func (m *Manager) runList(spec Spec) (Result, error) {
	cols := make([]index.ProjectCol, len(spec.Columns))
	for i, c := range spec.Columns {
		cols[i] = index.ProjectCol{Kind: c.Source.Kind, Key: c.Source.Key, Col: c.Source.Col}
	}

	sorts := make([]index.ProjectSort, 0, len(spec.OrderBy))
	for _, s := range spec.OrderBy {
		if s.Column < 0 || s.Column >= len(spec.Columns) {
			return Result{}, fmt.Errorf("query: order column %d out of range", s.Column)
		}
		sorts = append(sorts, index.ProjectSort{Index: s.Column, Desc: s.Desc, Numeric: s.Numeric})
	}

	rows, err := m.idx.ProjectRows(spec.Template, index.ProjectSpec{
		Cols:     cols,
		Filters:  toAggFilters(spec.Filters),
		Distinct: spec.Distinct,
		OrderBy:  sorts,
		Limit:    spec.Limit,
	})
	if err != nil {
		return Result{}, err
	}

	res := Result{Columns: headers(spec.Columns)}
	for _, r := range rows {
		cells := make([]Cell, len(r.Cells))
		for i, pc := range r.Cells {
			cells[i] = toCell(pc.Text, pc.Num.Valid, pc.Num.Float64)
		}
		res.Rows = append(res.Rows, cells)
	}
	res.Count = len(res.Rows)
	m.fillTotal(spec.Template, &res)
	return res, nil
}

// runGroup aggregates via AggregateRaw, then groups the raw rows by their
// dimension tuple in Go and counts the contributors. The result carries
// the group columns plus, when Count is set, the per-group count. Groups
// are ordered by key for determinism; Limit truncates.
func (m *Manager) runGroup(spec Spec) (Result, error) {
	dims := make([]index.AggDim, len(spec.GroupBy))
	groupHeaders := make([]string, len(spec.GroupBy))
	for i, gi := range spec.GroupBy {
		if gi < 0 || gi >= len(spec.Columns) {
			return Result{}, fmt.Errorf("query: group column %d out of range", gi)
		}
		s := spec.Columns[gi].Source
		dims[i] = index.AggDim{Kind: s.Kind, Key: s.Key, Col: s.Col}
		groupHeaders[i] = spec.Columns[gi].Header
	}

	raw, err := m.idx.AggregateRaw(spec.Template, dims, nil, toAggFilters(spec.Filters))
	if err != nil {
		return Result{}, err
	}

	counts := map[string]int{}
	tuples := map[string][]string{}
	var order []string
	for _, r := range raw {
		k := strings.Join(r.Dims, "\x1f")
		if _, seen := counts[k]; !seen {
			order = append(order, k)
			tuples[k] = append([]string(nil), r.Dims...)
		}
		counts[k]++
	}
	sort.Strings(order)

	res := Result{Columns: groupHeaders}
	if spec.Count {
		res.Columns = append(res.Columns, countHeader(spec))
	}
	for i, k := range order {
		if spec.Limit > 0 && i >= spec.Limit {
			break
		}
		cells := make([]Cell, 0, len(tuples[k])+1)
		for _, dv := range tuples[k] {
			cells = append(cells, Cell{Text: dv})
		}
		if spec.Count {
			n := float64(counts[k])
			cells = append(cells, Cell{Text: strconv.Itoa(counts[k]), Num: &n})
		}
		res.Rows = append(res.Rows, cells)
	}
	res.Count = len(res.Rows)
	m.fillTotal(spec.Template, &res)
	return res, nil
}

// fillTotal sets Result.Total from the template's form count. A failure
// here is non-fatal: the denominator is context, not the answer, so we
// leave it zero rather than fail an otherwise good query.
func (m *Manager) fillTotal(template string, res *Result) {
	if n, err := m.idx.TotalForms(template); err == nil {
		res.Total = n
	}
}

func headers(cols []Column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

func countHeader(spec Spec) string {
	if spec.CountHeader != "" {
		return spec.CountHeader
	}
	return defaultCountHeader
}

func toAggFilters(filters []Filter) []index.AggFilter {
	if len(filters) == 0 {
		return nil
	}
	out := make([]index.AggFilter, len(filters))
	for i, f := range filters {
		out[i] = index.AggFilter{
			Kind:  f.Source.Kind,
			Key:   f.Source.Key,
			Col:   f.Source.Col,
			Op:    f.Op,
			Value: f.Value,
		}
	}
	return out
}

func toCell(text string, hasNum bool, num float64) Cell {
	c := Cell{Text: text}
	if hasNum {
		v := num
		c.Num = &v
	}
	return c
}
