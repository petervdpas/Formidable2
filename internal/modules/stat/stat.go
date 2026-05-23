package stat

import (
	"fmt"
	"sort"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// Kind tags a Result with the chart family it naturally fits. Plugins
// switch on it to pick a renderer; the values are stable contract.
const (
	KindDistribution = "distribution"
	KindCrosstab     = "crosstab"
	KindScalarStats  = "scalar_stats"
	KindTimeSeries   = "timeseries"
)

// Series is one named row of numbers aligned to a Result's Categories.
type Series struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

// Result is the chart-neutral output of every stat query. Categories
// are the x-axis labels; Series are the aligned value rows; Scalars
// carries single-number stats that don't fit the grid (count, avg,
// median, ...). Total is the form denominator, so a plugin can render
// percentages without a second call.
type Result struct {
	Kind       string             `json:"kind"`
	Categories []string           `json:"categories,omitempty"`
	Series     []Series           `json:"series,omitempty"`
	Scalars    map[string]float64 `json:"scalars,omitempty"`
	Total      int                `json:"total"`
}

// Index is the narrow slice of index.Manager the stat module needs.
// *index.Manager satisfies it; tests pass a fake.
type Index interface {
	TotalForms(template string) (int, error)
	ValueDistribution(template, fieldKey string, col *int) ([]index.Bucket, error)
	NumericValues(template, fieldKey string, col *int) ([]float64, error)
	FacetDistribution(template, facetKey string) ([]index.Bucket, error)
	FacetCross(template, keyA, keyB string) ([]index.CrossCell, error)
	DateSeries(template, fieldKey string, col *int, period string) ([]index.Bucket, error)
}

// Manager turns index aggregates into chart-neutral Results.
type Manager struct {
	idx Index
}

func NewManager(idx Index) *Manager { return &Manager{idx: idx} }

// TotalForms is the form-count denominator for percentage stats.
func (m *Manager) TotalForms(template string) (int, error) {
	return m.idx.TotalForms(template)
}

// Distribution counts forms by a field's value (col nil) or a table
// column's value (col set). Single-series "count" over the distinct
// values.
func (m *Manager) Distribution(template, fieldKey string, col *int) (*Result, error) {
	buckets, err := m.idx.ValueDistribution(template, fieldKey, col)
	if err != nil {
		return nil, err
	}
	return m.bucketsToResult(template, buckets)
}

// FacetDistribution counts set forms by a facet's selected option.
func (m *Manager) FacetDistribution(template, facetKey string) (*Result, error) {
	buckets, err := m.idx.FacetDistribution(template, facetKey)
	if err != nil {
		return nil, err
	}
	return m.bucketsToResult(template, buckets)
}

// TimeSeries buckets a date field / column by period ("year" | "month"
// | "day") and counts forms per bucket, chronologically.
func (m *Manager) TimeSeries(template, fieldKey string, col *int, period string) (*Result, error) {
	buckets, err := m.idx.DateSeries(template, fieldKey, col, period)
	if err != nil {
		return nil, err
	}
	res, err := m.bucketsToResult(template, buckets)
	if err != nil {
		return nil, err
	}
	res.Kind = KindTimeSeries
	return res, nil
}

// NumericStats reduces a numeric field / column to summary scalars
// (min/max/sum/avg/median/stddev[/percentile]). percentile may be nil.
func (m *Manager) NumericStats(template, fieldKey string, col *int, percentile *float64) (*Result, error) {
	vals, err := m.idx.NumericValues(template, fieldKey, col)
	if err != nil {
		return nil, err
	}
	total, err := m.idx.TotalForms(template)
	if err != nil {
		return nil, err
	}
	res := &Result{Kind: KindScalarStats, Total: total, Scalars: map[string]float64{"count": 0}}
	s, ok := Summarize(vals, percentile)
	if !ok {
		return res, nil
	}
	res.Scalars = map[string]float64{
		"count":  float64(s.Count),
		"min":    s.Min,
		"max":    s.Max,
		"sum":    s.Sum,
		"avg":    s.Avg,
		"median": s.Median,
		"stddev": s.Stddev,
	}
	if s.Percentile != nil {
		res.Scalars["percentile"] = *s.Percentile
	}
	return res, nil
}

// CrossTab is the pair-combination matrix between two facets: categories
// are the distinct A-options; each distinct B-option becomes a series,
// values aligned to the categories. Zero-filled where a pair is absent.
func (m *Manager) CrossTab(template, keyA, keyB string) (*Result, error) {
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

	series := make([]Series, 0, len(bLabels))
	for _, b := range bLabels {
		vals := make([]float64, len(aLabels))
		for i, a := range aLabels {
			vals[i] = float64(counts[a][b])
		}
		series = append(series, Series{Name: b, Values: vals})
	}
	return &Result{Kind: KindCrosstab, Categories: aLabels, Series: series, Total: total}, nil
}

// bucketsToResult is the shared distribution shaper: labels → categories,
// counts → one "count" series, plus the form total for percentages.
func (m *Manager) bucketsToResult(template string, buckets []index.Bucket) (*Result, error) {
	total, err := m.idx.TotalForms(template)
	if err != nil {
		return nil, fmt.Errorf("stat: total forms: %w", err)
	}
	cats := make([]string, len(buckets))
	vals := make([]float64, len(buckets))
	for i, b := range buckets {
		cats[i] = b.Label
		vals[i] = float64(b.Count)
	}
	return &Result{
		Kind:       KindDistribution,
		Categories: cats,
		Series:     []Series{{Name: "count", Values: vals}},
		Total:      total,
	}, nil
}
