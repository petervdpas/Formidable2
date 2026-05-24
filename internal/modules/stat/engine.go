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

// Evaluate runs a StatConfig against the index and shapes the result into
// a Grid. Scalar field and facet sources only; table-column sources are
// deferred and rejected with a clear error.
func (m *Manager) Evaluate(template string, cfg StatConfig) (*Grid, error) {
	if len(cfg.Measures) == 0 {
		return nil, fmt.Errorf("stat: config has no measures")
	}

	dims := make([]index.AggDim, len(cfg.Dimensions))
	for i, d := range cfg.Dimensions {
		if d.Source.Column != "" {
			return nil, fmt.Errorf("stat: table-column dimension %q[%q] not yet supported by the engine", d.Source.Key, d.Source.Column)
		}
		kind := "field"
		if d.Source.Kind == SourceFacet {
			kind = "facet"
		}
		dims[i] = index.AggDim{Kind: kind, Key: d.Source.Key, DateWidth: binWidth(d.Bin)}
	}

	// Distinct numeric source keys for the reducing measures (count has none).
	numIdx := map[string]int{}
	var numKeys []string
	for _, ms := range cfg.Measures {
		if ms.Source == nil {
			continue
		}
		if ms.Source.Column != "" {
			return nil, fmt.Errorf("stat: table-column measure source %q[%q] not yet supported", ms.Source.Key, ms.Source.Column)
		}
		if ms.Source.Kind != SourceField {
			return nil, fmt.Errorf("stat: measure source must be a field")
		}
		if _, ok := numIdx[ms.Source.Key]; !ok {
			numIdx[ms.Source.Key] = len(numKeys)
			numKeys = append(numKeys, ms.Source.Key)
		}
	}

	rows, err := m.idx.AggregateRaw(template, dims, numKeys)
	if err != nil {
		return nil, err
	}
	total, err := m.idx.TotalForms(template)
	if err != nil {
		return nil, err
	}

	nd := len(cfg.Dimensions)

	// Pass 1: distinct, sorted labels per axis.
	axes := make([]GridAxis, nd)
	labelIdx := make([]map[string]int, nd)
	for i := range axes {
		axes[i] = GridAxis{Source: sourceLabel(cfg.Dimensions[i].Source, cfg.Dimensions[i].Bin)}
		seen := map[string]bool{}
		for _, r := range rows {
			if !seen[r.Dims[i]] {
				seen[r.Dims[i]] = true
				axes[i].Labels = append(axes[i].Labels, r.Dims[i])
			}
		}
		sort.Strings(axes[i].Labels)
		labelIdx[i] = make(map[string]int, len(axes[i].Labels))
		for idx, lbl := range axes[i].Labels {
			labelIdx[i][lbl] = idx
		}
	}

	// Pass 2: group rows by coordinate tuple, collecting count + per-source values.
	type group struct {
		coords []int
		count  int
		nums   [][]float64 // aligned to numKeys
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
			g = &group{coords: coords, nums: make([][]float64, len(numKeys))}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
		for j := range numKeys {
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
			vals[i] = reduceMeasure(ms, g.count, g.nums, numIdx)
		}
		cells = append(cells, GridCell{Coords: g.coords, Values: vals})
	}

	return &Grid{Axes: axes, Measures: measures, Cells: cells, Total: total}, nil
}

func reduceMeasure(ms Measure, count int, nums [][]float64, numIdx map[string]int) float64 {
	if ms.Op == OpCount {
		return float64(count)
	}
	vals := nums[numIdx[ms.Source.Key]]
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
