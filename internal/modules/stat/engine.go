package stat

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// Grid is the rank-N output of evaluating a StatConfig: one Axis per
// dimension (in declared order), the measure labels, and sparse Cells
// (a coordinate tuple into the axes plus one value per measure). Total is
// the form-count denominator for percentages. No dimensions => one cell
// with empty coords (a rank-0 scalar).
type Grid struct {
	Axes     []GridAxis `json:"axes"`
	Measures []string   `json:"measures"`
	Cells    []GridCell `json:"cells"`
	Total    int        `json:"total"`
}

// GridAxis is one dimension: a source label and its distinct category
// ticks (sorted for stable output).
type GridAxis struct {
	Source string   `json:"source"`
	Labels []string `json:"labels"`
}

// GridCell is one populated coordinate: indices into each axis and the
// value of each measure (aligned to Grid.Measures).
type GridCell struct {
	Coords []int     `json:"coords"`
	Values []float64 `json:"values"`
}

// EvaluateDSL parses a statistical-DSL string and evaluates it against the
// index. Convenience for callers that hold the stored DSL string (the
// Wails service, the Lua binding) rather than a built Config.
func (m *Manager) EvaluateDSL(template, dsl string) (*Grid, error) {
	cfg, err := Parse(dsl)
	if err != nil {
		return nil, err
	}
	return m.Evaluate(template, cfg)
}

// Evaluate runs a StatConfig against the index and shapes the result into
// a Grid. Supports scalar field, facet and (a single) table-column source;
// see the fan-out guard below for the combinations rejected to avoid
// over-counting.
func (m *Manager) Evaluate(template string, cfg StatConfig) (*Grid, error) {
	if len(cfg.Measures) == 0 {
		return nil, fmt.Errorf("stat: config has no measures")
	}

	// Fan-out guard: a table-column source joins one row per cell (many per
	// form). More than one such source would multiply (cartesian) and
	// over-count; and a numeric measure over a scalar alongside a
	// table-column dimension would repeat the scalar per cell. Both are
	// rejected; a single table-column dimension with count() is exact.
	tableSrc := 0
	hasTableDim := false
	for _, d := range cfg.Dimensions {
		if d.Source.Column != "" {
			tableSrc++
			hasTableDim = true
		}
	}
	for _, ms := range cfg.Measures {
		if ms.Source != nil && ms.Source.Column != "" {
			tableSrc++
		}
	}
	for _, f := range cfg.Filters {
		if f.Source.Column != "" {
			tableSrc++
		}
	}
	if tableSrc > 1 {
		return nil, fmt.Errorf("stat: a statistic may use at most one table-column source (more would over-count)")
	}
	if hasTableDim {
		for _, ms := range cfg.Measures {
			if ms.Op != OpCount && ms.Op != OpRecords {
				return nil, fmt.Errorf("stat: a table-column dimension supports only count()/records() (numeric measures would over-count)")
			}
		}
	}

	// records() counts distinct contributing forms per category, so the
	// reducer must track form identity per group (skipped otherwise).
	needRecords := false
	for _, ms := range cfg.Measures {
		if ms.Op == OpRecords {
			needRecords = true
			break
		}
	}

	// resolveCol turns a table-column source's column key into its
	// positional index; scalar/facet sources resolve to nil.
	resolveCol := func(s SourceRef) (*int, error) {
		if s.Column == "" {
			return nil, nil
		}
		if m.cols == nil {
			return nil, fmt.Errorf("stat: table-column source %q[%q] needs a column resolver", s.Key, s.Column)
		}
		idx, ok := m.cols.ColumnIndex(template, s.Key, s.Column)
		if !ok {
			return nil, fmt.Errorf("stat: table column %q[%q] not found", s.Key, s.Column)
		}
		return &idx, nil
	}

	dims := make([]index.AggDim, len(cfg.Dimensions))
	for i, d := range cfg.Dimensions {
		kind := "field"
		if d.Source.Kind == SourceFacet {
			kind = "facet"
		}
		col, err := resolveCol(d.Source)
		if err != nil {
			return nil, err
		}
		dims[i] = index.AggDim{Kind: kind, Key: d.Source.Key, Col: col, DateWidth: binWidth(d.Bin)}
	}

	// Distinct numeric sources for the reducing measures, keyed by
	// (key,column) so two columns of one table field don't collide.
	numKeyFor := func(s *SourceRef) string { return s.Key + "\x00" + s.Column }
	numIdx := map[string]int{}
	var nums []index.AggNum
	for _, ms := range cfg.Measures {
		if ms.Source == nil {
			continue
		}
		if ms.Source.Kind != SourceField {
			return nil, fmt.Errorf("stat: measure source must be a field")
		}
		k := numKeyFor(ms.Source)
		if _, ok := numIdx[k]; ok {
			continue
		}
		col, err := resolveCol(*ms.Source)
		if err != nil {
			return nil, err
		}
		numIdx[k] = len(nums)
		nums = append(nums, index.AggNum{Key: ms.Source.Key, Col: col})
	}

	filters := make([]index.AggFilter, 0, len(cfg.Filters))
	for _, f := range cfg.Filters {
		kind := "field"
		if f.Source.Kind == SourceFacet {
			kind = "facet"
		}
		if kind == "facet" && comparisonOps[f.Op] {
			return nil, fmt.Errorf("stat: comparison filter %q needs a numeric field, not a facet", f.Op)
		}
		col, err := resolveCol(f.Source)
		if err != nil {
			return nil, err
		}
		filters = append(filters, index.AggFilter{Kind: kind, Key: f.Source.Key, Col: col, Op: string(f.Op), Value: f.Value})
	}

	rows, err := m.idx.AggregateRaw(template, dims, nums, filters)
	if err != nil {
		return nil, err
	}
	total, err := m.idx.TotalForms(template)
	if err != nil {
		return nil, err
	}

	nd := len(cfg.Dimensions)

	// Pass 1: labels per axis. When the dimension has a fixed category set
	// (a facet / choice source, via SourceOptions), use the full defined
	// order and append any present-but-undefined values (stale/unset) after
	// it. Otherwise, the sorted distinct present values. Date-binned
	// dimensions are always present-values (their buckets aren't a fixed
	// set). This is what surfaces zero-count categories instead of dropping
	// them.
	axes := make([]GridAxis, nd)
	labelIdx := make([]map[string]int, nd) // keyed by stored value -> axis index
	for i := range axes {
		dim := cfg.Dimensions[i]
		axes[i] = GridAxis{Source: sourceLabel(dim.Source, dim.Bin)}
		labelIdx[i] = map[string]int{}

		present := map[string]bool{}
		for _, r := range rows {
			present[r.Dims[i]] = true
		}

		// add appends one category: index it by its stored value, display
		// its label.
		add := func(value, label string) {
			if _, dup := labelIdx[i][value]; dup {
				return
			}
			labelIdx[i][value] = len(axes[i].Labels)
			axes[i].Labels = append(axes[i].Labels, label)
		}

		var fixed []CategoryOption
		if m.opts != nil && dim.Bin == BinNone {
			if defined, ok := m.opts.DimensionLabels(template, dim.Source); ok {
				fixed = defined
			}
		}
		if fixed != nil {
			for _, o := range fixed {
				add(o.Value, o.Label)
			}
			// Present-but-undefined stored values (stale/unset) appended,
			// shown as themselves.
			extra := make([]string, 0)
			for v := range present {
				if _, known := labelIdx[i][v]; !known {
					extra = append(extra, v)
				}
			}
			sort.Strings(extra)
			for _, v := range extra {
				add(v, v)
			}
		} else {
			vals := make([]string, 0, len(present))
			for v := range present {
				vals = append(vals, v)
			}
			sort.Strings(vals)
			for _, v := range vals {
				add(v, v)
			}
		}
	}

	// Pass 2: group rows by coordinate tuple, collecting count + per-source values.
	type group struct {
		coords []int
		count  int
		forms  map[string]struct{} // distinct contributing forms; nil unless records() asked
		nums   [][]float64         // aligned to numKeys
	}
	groups := map[string]*group{}
	var order []string
	for _, r := range rows {
		coords := make([]int, nd)
		for i := range nd {
			coords[i] = labelIdx[i][r.Dims[i]]
		}
		key := coordKey(coords)
		g := groups[key]
		if g == nil {
			g = &group{coords: coords, nums: make([][]float64, len(nums))}
			if needRecords {
				g.forms = map[string]struct{}{}
			}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
		if needRecords {
			g.forms[r.Form] = struct{}{}
		}
		for j := range nums {
			if r.Nums[j].Valid {
				g.nums[j] = append(g.nums[j], r.Nums[j].Float64)
			}
		}
	}

	measures := make([]string, len(cfg.Measures))
	for i, ms := range cfg.Measures {
		measures[i] = measureLabel(ms)
	}

	cells := make([]GridCell, 0, len(order))
	for _, key := range order {
		g := groups[key]
		vals := make([]float64, len(cfg.Measures))
		for i, ms := range cfg.Measures {
			if ms.Op == OpCount {
				vals[i] = float64(g.count)
				continue
			}
			if ms.Op == OpRecords {
				vals[i] = float64(len(g.forms))
				continue
			}
			vals[i] = reduceNumeric(ms, g.nums[numIdx[numKeyFor(ms.Source)]])
		}
		cells = append(cells, GridCell{Coords: g.coords, Values: vals})
	}

	grid := &Grid{Axes: axes, Measures: measures, Cells: cells, Total: total}

	// Top-N: cap a (typically high-cardinality) dimension to its biggest
	// categories by the first measure, dropping the tail. Total is left as
	// the full form count.
	for i, d := range cfg.Dimensions {
		if d.Top > 0 {
			applyTopN(grid, i, min(d.Top, 20))
		}
	}
	return grid, nil
}

// applyTopN keeps the n highest categories on axis `ax` (ranked by the
// first measure's total, descending; ties keep original order), reindexes
// the axis and cells, and drops cells in the removed categories.
func applyTopN(g *Grid, ax, n int) {
	labels := g.Axes[ax].Labels
	if n >= len(labels) || len(labels) == 0 {
		return
	}
	totals := make([]float64, len(labels))
	for _, c := range g.Cells {
		if len(c.Values) > 0 {
			totals[c.Coords[ax]] += c.Values[0]
		}
	}
	order := make([]int, len(labels))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return totals[order[a]] > totals[order[b]] })

	remap := make(map[int]int, n)
	newLabels := make([]string, n)
	for newIdx, oldIdx := range order[:n] {
		remap[oldIdx] = newIdx
		newLabels[newIdx] = labels[oldIdx]
	}
	g.Axes[ax].Labels = newLabels

	kept := make([]GridCell, 0, len(g.Cells))
	for _, c := range g.Cells {
		ni, ok := remap[c.Coords[ax]]
		if !ok {
			continue
		}
		coords := append([]int(nil), c.Coords...)
		coords[ax] = ni
		kept = append(kept, GridCell{Coords: coords, Values: c.Values})
	}
	g.Cells = kept
}

func reduceNumeric(ms Measure, vals []float64) float64 {
	s, ok := Summarize(vals, ms.Arg)
	if !ok {
		return 0
	}
	switch ms.Op {
	case OpSum:
		return s.Sum
	case OpAvg:
		return s.Avg
	case OpMin:
		return s.Min
	case OpMax:
		return s.Max
	case OpMedian:
		return s.Median
	case OpStddev:
		return s.Stddev
	case OpPercentile:
		if s.Percentile != nil {
			return *s.Percentile
		}
	}
	return 0
}

func binWidth(b Bin) int {
	switch b {
	case BinYear:
		return 4
	case BinMonth:
		return 7
	case BinDay:
		return 10
	}
	return 0
}

func srcKey(s *SourceRef) string {
	if s == nil {
		return ""
	}
	if s.Column != "" {
		return s.Key + "." + s.Column
	}
	return s.Key
}

func sourceLabel(s SourceRef, bin Bin) string {
	out := srcKey(&s)
	if bin != BinNone {
		out += "@" + string(bin)
	}
	return out
}

func measureLabel(ms Measure) string {
	if ms.Op == OpCount {
		return "count"
	}
	if ms.Op == OpRecords {
		return "records"
	}
	if ms.Op == OpPercentile && ms.Arg != nil {
		return "p" + formatNum(*ms.Arg) + "(" + srcKey(ms.Source) + ")"
	}
	return string(ms.Op) + "(" + srcKey(ms.Source) + ")"
}

func coordKey(coords []int) string {
	parts := make([]string, len(coords))
	for i, c := range coords {
		parts[i] = strconv.Itoa(c)
	}
	return strings.Join(parts, ",")
}
