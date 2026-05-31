package datacore

import (
	"sort"
	"strconv"
	"strings"
)

// Anomaly is a value present where a reduction expected a number (or date)
// but the string would not coerce. It is surfaced, not dropped: a value
// that refuses the type an operation asks for is a signal, not noise.
type Anomaly struct {
	ID    string
	Field string
	Value string
}

// Aggregate is the result of a numeric reduction over a field: the one-pass
// summary, the raw coercible values, and any value that refused to coerce.
// With no numeric values N is 0 and Min/Max/Mean are 0.
//
// Values holds the coerced numbers in working-set order so a caller can run
// the statistics SQLite has no built-ins for (median, stddev, percentile)
// without a second pass. It is exactly the set that fed N/Sum: blanks are
// excluded, non-coercible values go to Anomalies. len(Values) == N.
type Aggregate struct {
	N         int
	Sum       float64
	Min       float64
	Max       float64
	Mean      float64
	Values    []float64
	Anomalies []Anomaly
}

// Aggregate reduces a numeric field over the working set, coercing each
// value on demand: the tensor stores strings, the operation reapplies the
// type. Blank cells are absence. A non-blank value that will not parse is
// recorded as an Anomaly (sorted by identity) rather than dropped or
// counted as zero, so a typed field holding junk stays visible.
func (p *Perspective) Aggregate(field string) Aggregate {
	var a Aggregate
	f, ok := p.t.fax.lookup(field)
	if !ok {
		return a
	}
	first := true
	for _, i := range p.identities() {
		v, _, ok := p.t.at(i, f, p.scope)
		if !ok || v == "" {
			continue
		}
		n, err := parseNum(v)
		if err != nil {
			a.Anomalies = append(a.Anomalies, Anomaly{ID: p.t.iax.label(i), Field: field, Value: v})
			continue
		}
		a.N++
		a.Sum += n
		a.Values = append(a.Values, n)
		if first || n < a.Min {
			a.Min = n
		}
		if first || n > a.Max {
			a.Max = n
		}
		first = false
	}
	if a.N > 0 {
		a.Mean = a.Sum / float64(a.N)
	}
	sort.Slice(a.Anomalies, func(x, y int) bool { return a.Anomalies[x].ID < a.Anomalies[y].ID })
	return a
}

func parseNum(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// RootSummary is one root's loop summary: its identity, the loop length
// (sub-identities it links under the link field), and the numeric reduction
// of a value field over only that root's rows.
type RootSummary struct {
	ID   string
	Rows int
	Agg  Aggregate
}

// Summarize reduces per root: for each identity it aggregates valueField
// over only the rows that root references under linkField, landing the
// result back on the root. Form X's total is computed over X's own loop
// rows, never pooled across forms. Agg is left zero when valueField is ""
// (a plain count summary). Roots with no rows still appear, Rows 0.
// Ordered by root.
func (p *Perspective) Summarize(linkField, valueField string) []RootSummary {
	lf, ok := p.t.fax.lookup(linkField)
	if !ok {
		return nil
	}
	out := make([]RootSummary, 0)
	for _, root := range p.identities() {
		rows := p.t.refsFrom(root, lf)
		s := RootSummary{ID: p.t.iax.label(root), Rows: len(rows)}
		if valueField != "" && len(rows) > 0 {
			child := &Perspective{t: p.t, scope: p.scope, ids: rows}
			s.Agg = child.Aggregate(valueField)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}
