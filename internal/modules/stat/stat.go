package stat

import (
	"fmt"
	"sort"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// Every stat query returns the same shape - a rank-N *Grid (see
// engine.go) - so the frontend has one renderer family and the data
// API is uniform. The aggregation differs per query (these convenience
// builders use the index's purpose-built aggregates), but the output is
// always a Grid: rank-1 for distributions / facet counts / time series,
// rank-2 for cross-tabs, rank-0 (one cell, many measures) for numeric
// summaries.

// Index is the read surface the stat engine computes through.
type Index interface {
	TotalForms(template string) (int, error)
	ValueDistribution(template, fieldKey string, col *int) ([]index.Bucket, error)
	NumericValues(template, fieldKey string, col *int) ([]float64, error)
	FacetDistribution(template, facetKey string) ([]index.Bucket, error)
	FacetCross(template, keyA, keyB string) ([]index.CrossCell, error)
	DateSeries(template, fieldKey string, col *int, period string) ([]index.Bucket, error)
	AggregateRaw(template string, dims []index.AggDim, nums []index.AggNum, filters []index.AggFilter) ([]index.StatRawRow, error)
}

// ColumnResolver maps a table field's column value-key to its positional
// form_values.col index, so the engine can turn a table-column DSL source
// (F["table"]["colKey"]) into the indexed column. ok=false when unknown.
type ColumnResolver interface {
	ColumnIndex(template, fieldKey, columnKey string) (int, bool)
}

// CategoryOption is one fixed category of a dimension source: Value is
// what the index stores (the group-by key); Label is what to display. For
// a facet the two are equal (it stores its option label); for a choice
// field Value is the option value and Label its human caption.
type CategoryOption struct {
	Value string
	Label string
}

// SourceOptions supplies the full, ordered category set for a dimension
// source that has a fixed one (a facet / choice field), so the grid shows
// every defined category - including zero-count ones - in the author's
// order, displayed by Label but grouped by Value. Returns ok=false for
// open-ended sources (dates, numbers, free text), which fall back to the
// sorted present values.
type SourceOptions interface {
	DimensionLabels(template string, src SourceRef) (opts []CategoryOption, ok bool)
}

// Manager turns index aggregates into chart-neutral Results and grids.
type Manager struct {
	idx  Index
	opts SourceOptions
	cols ColumnResolver
}

func NewManager(idx Index) *Manager { return &Manager{idx: idx} }

// SetSourceOptions wires the optional fixed-category resolver used by
// Evaluate to give facet/choice dimensions their full ordered axis.
func (m *Manager) SetSourceOptions(o SourceOptions) { m.opts = o }

// SetColumnResolver wires the optional table-column key->index resolver
// used by Evaluate to support table-column dimension/measure sources.
func (m *Manager) SetColumnResolver(c ColumnResolver) { m.cols = c }

// TotalForms is the form-count denominator for percentage stats.
func (m *Manager) TotalForms(template string) (int, error) {
	return m.idx.TotalForms(template)
}

// Distribution counts forms by a field's value (col nil) or a table
// column's value (col set), as a rank-1 count Grid over the distinct
// values.
func (m *Manager) Distribution(template, fieldKey string, col *int) (*Grid, error) {
	buckets, err := m.idx.ValueDistribution(template, fieldKey, col)
	if err != nil {
		return nil, err
	}
	return m.bucketsToGrid(template, fieldKey, buckets)
}

// FacetDistribution counts set forms by a facet's selected option. The
// axis source is the facet key, so a renderer can color categories with
// the facet's authored option colors.
func (m *Manager) FacetDistribution(template, facetKey string) (*Grid, error) {
	buckets, err := m.idx.FacetDistribution(template, facetKey)
	if err != nil {
		return nil, err
	}
	return m.bucketsToGrid(template, facetKey, buckets)
}

// TimeSeries buckets a date field / column by period ("year" | "month"
// | "day") and counts forms per bucket, chronologically, as a rank-1
// Grid.
func (m *Manager) TimeSeries(template, fieldKey string, col *int, period string) (*Grid, error) {
	buckets, err := m.idx.DateSeries(template, fieldKey, col, period)
	if err != nil {
		return nil, err
	}
	return m.bucketsToGrid(template, fieldKey, buckets)
}

// NumericStats reduces a numeric field / column to summary measures
// (count/min/max/sum/avg/median/stddev[/percentile]) as a rank-0 Grid:
// no axes, one cell whose values align to the measure names. percentile
// may be nil.
func (m *Manager) NumericStats(template, fieldKey string, col *int, percentile *float64) (*Grid, error) {
	vals, err := m.idx.NumericValues(template, fieldKey, col)
	if err != nil {
		return nil, err
	}
	total, err := m.idx.TotalForms(template)
	if err != nil {
		return nil, err
	}
	measures := []string{"count"}
	values := []float64{0}
	if s, ok := Summarize(vals, percentile); ok {
		measures = []string{"count", "min", "max", "sum", "avg", "median", "stddev"}
		values = []float64{float64(s.Count), s.Min, s.Max, s.Sum, s.Avg, s.Median, s.Stddev}
		if s.Percentile != nil {
			measures = append(measures, "percentile")
			values = append(values, *s.Percentile)
		}
	}
	g := &Grid{
		Axes:     []GridAxis{},
		Measures: measures,
		Cells:    []GridCell{{Coords: []int{}, Values: values}},
		Total:    total,
	}
	addPercents(g, PctDistribution, float64(g.Total))
	return g, nil
}

// CrossTab is the pair-combination matrix between two facets, as a
// rank-2 count Grid: axis 0 = the distinct A-options, axis 1 = the
// distinct B-options; one cell per populated (A,B) pair.
func (m *Manager) CrossTab(template, keyA, keyB string) (*Grid, error) {
	cells, err := m.idx.FacetCross(template, keyA, keyB)
	if err != nil {
		return nil, err
	}
	total, err := m.idx.TotalForms(template)
	if err != nil {
		return nil, err
	}

	aLabels, bLabels := []string{}, []string{}
	aSeen, bSeen := map[string]struct{}{}, map[string]struct{}{}
	counts := map[string]map[string]int{}
	for _, c := range cells {
		if _, ok := aSeen[c.A]; !ok {
			aSeen[c.A] = struct{}{}
			aLabels = append(aLabels, c.A)
		}
		if _, ok := bSeen[c.B]; !ok {
			bSeen[c.B] = struct{}{}
			bLabels = append(bLabels, c.B)
		}
		if counts[c.A] == nil {
			counts[c.A] = map[string]int{}
		}
		counts[c.A][c.B] = c.Count
	}
	sort.Strings(aLabels)
	sort.Strings(bLabels)

	aIdx := make(map[string]int, len(aLabels))
	for i, a := range aLabels {
		aIdx[a] = i
	}
	bIdx := make(map[string]int, len(bLabels))
	for i, b := range bLabels {
		bIdx[b] = i
	}
	grid := make([]GridCell, 0, len(cells))
	for a, row := range counts {
		for b, n := range row {
			if n == 0 {
				continue
			}
			grid = append(grid, GridCell{Coords: []int{aIdx[a], bIdx[b]}, Values: []float64{float64(n)}})
		}
	}
	g := &Grid{
		Axes:     []GridAxis{{Source: keyA, Labels: aLabels}, {Source: keyB, Labels: bLabels}},
		Measures: []string{"count"},
		Cells:    grid,
		Total:    total,
	}
	addPercents(g, PctDistribution, float64(g.Total))
	return g, nil
}

// bucketsToGrid is the shared rank-1 shaper: labels → axis ticks, counts
// → one "count" measure, plus the form total for percentages. `source`
// is the axis source (field or facet key) the renderer keys colors on.
func (m *Manager) bucketsToGrid(template, source string, buckets []index.Bucket) (*Grid, error) {
	total, err := m.idx.TotalForms(template)
	if err != nil {
		return nil, fmt.Errorf("stat: total forms: %w", err)
	}
	labels := make([]string, len(buckets))
	cells := make([]GridCell, 0, len(buckets))
	for i, b := range buckets {
		labels[i] = b.Label
		cells = append(cells, GridCell{Coords: []int{i}, Values: []float64{float64(b.Count)}})
	}
	g := &Grid{
		Axes:     []GridAxis{{Source: source, Labels: labels}},
		Measures: []string{"count"},
		Cells:    cells,
		Total:    total,
	}
	addPercents(g, PctDistribution, float64(g.Total))
	return g, nil
}
