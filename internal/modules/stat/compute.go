// Package stat computes chart-neutral statistics over a profile's indexed
// form data. It owns no storage: every number comes from the index module's
// aggregate queries (form_values / form_facets), a derived cache of the
// canonical .meta.json files. Output is transport- and chart-library-neutral;
// plugins shape it into a concrete chart spec.
package stat

import (
	"math"
	"sort"
)

// Summary is the numeric reduction of a value column. Percentile is
// optional (nil input omits it) so "no percentile requested" is
// distinct from "the p-value happened to be zero".
type Summary struct {
	Count      int      `json:"count"`
	Min        float64  `json:"min"`
	Max        float64  `json:"max"`
	Sum        float64  `json:"sum"`
	Avg        float64  `json:"avg"`
	Median     float64  `json:"median"`
	Stddev     float64  `json:"stddev"`
	Percentile *float64 `json:"percentile,omitempty"`
}

// Summarize reduces a float column to a Summary. Returns ok=false for
// an empty input so callers can distinguish "no data" from "all zero".
// percentile is in [0,100] (clamped); nil skips it. Sample stddev
// (n-1) matches the render module's existing stats helper.
func Summarize(vals []float64, percentile *float64) (Summary, bool) {
	if len(vals) == 0 {
		return Summary{}, false
	}
	clean := make([]float64, len(vals))
	copy(clean, vals)
	sort.Float64s(clean)

	n := len(clean)
	sum := 0.0
	for _, x := range clean {
		sum += x
	}
	avg := sum / float64(n)

	mid := n / 2
	median := clean[mid]
	if n%2 == 0 {
		median = (clean[mid-1] + clean[mid]) / 2
	}

	stddev := 0.0
	if n > 1 {
		variance := 0.0
		for _, x := range clean {
			variance += (x - avg) * (x - avg)
		}
		stddev = math.Sqrt(variance / float64(n-1))
	}

	out := Summary{
		Count:  n,
		Min:    clean[0],
		Max:    clean[n-1],
		Sum:    sum,
		Avg:    avg,
		Median: median,
		Stddev: stddev,
	}

	if percentile != nil {
		p := math.Max(0, math.Min(100, *percentile))
		idx := (p / 100) * float64(n-1)
		lo := int(math.Floor(idx))
		hi := int(math.Ceil(idx))
		var pv float64
		if lo == hi {
			pv = clean[lo]
		} else {
			w := idx - float64(lo)
			pv = clean[lo]*(1-w) + clean[hi]*w
		}
		out.Percentile = &pv
	}
	return out, true
}
