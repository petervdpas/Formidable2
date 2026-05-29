package datacore

import (
	"sort"
	"strings"
	"time"
)

// Series is a date histogram: counts bucketed by a period (year/month/day),
// plus any value that would not coerce to a date.
type Series struct {
	Period    string
	Buckets   []Bucket
	Anomalies []Anomaly
}

var dateLayouts = []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func normalizePeriod(period string) string {
	if period == "year" || period == "day" {
		return period
	}
	return "month"
}

func bucketDate(t time.Time, period string) string {
	switch period {
	case "year":
		return t.Format("2006")
	case "day":
		return t.Format("2006-01-02")
	default:
		return t.Format("2006-01")
	}
}

// DateSeries reduces along the I mode into a date histogram: each identity's
// value at field is coerced to a date on demand and bucketed to the period
// (year/month/day, defaulting to month). Blank cells are absence; a non-blank
// value that will not parse as a date is surfaced as an Anomaly, not
// truncated into a bogus bucket. Buckets are sorted (chronological for ISO
// dates, since lexical and chronological coincide).
func (p *Perspective) DateSeries(field, period string) Series {
	s := Series{Period: normalizePeriod(period)}
	f, ok := p.t.fax.lookup(field)
	if !ok {
		return s
	}
	counts := map[string]int{}
	for _, i := range p.identities() {
		v, _, ok := p.t.at(i, f, p.scope)
		if !ok || v == "" {
			continue
		}
		d, ok := parseDate(v)
		if !ok {
			s.Anomalies = append(s.Anomalies, Anomaly{ID: p.t.iax.label(i), Field: field, Value: v})
			continue
		}
		counts[bucketDate(d, s.Period)]++
	}
	for b, n := range counts {
		s.Buckets = append(s.Buckets, Bucket{Value: b, Count: n})
	}
	sort.Slice(s.Buckets, func(a, b int) bool { return s.Buckets[a].Value < s.Buckets[b].Value })
	sort.Slice(s.Anomalies, func(a, b int) bool { return s.Anomalies[a].ID < s.Anomalies[b].ID })
	return s
}
