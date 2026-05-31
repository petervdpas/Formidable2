// Package monitor is the generic observation surface over Formidable's
// internal event streams. The Manager runs typed Query values against
// any registered Source (journal, app log, request log) and returns a
// typed Result.
//
//   - Sources project their native records into Event{Ts, Kind, Dims, Value}.
//   - Query{Filter, GroupBy, Bin, Agg} is a typed pipeline. No DSL.
//   - Result feeds chart components and round-trips across the Wails bridge.
//
// New sources slot in by implementing Source. The HTTP handler at
// /api/monitor/* exposes the same Query shape as JSON for external consumers.
package monitor

import "time"

// Event is the canonical observation record. Dims are free-form
// key/value strings consumers filter and group by. Value is the
// per-event numeric contribution to non-count aggregations (default 1.0).
type Event struct {
	Ts    time.Time
	Kind  string
	Dims  map[string]string
	Value float64
}

// Source is a stream of Events bounded by a time range. Implementations
// must respect [from, to) (event.Ts >= from, event.Ts < to). A zero
// from and to means "all".
type Source interface {
	// Name is the identifier Query.Source selects against. Stable.
	Name() string

	// Kind is a coarse label for the Source's event domain, used only
	// by ListSources for UI groupings ("journal", "log", "request").
	Kind() string

	// Dims is the set of dimension names this Source emits, in stable order.
	Dims() []string

	// Events yields events in [from, to). May stream from disk; callers
	// must consume promptly.
	Events(from, to time.Time) []Event
}

// Aggregator reduces multiple Events with the same (groupKey, binStart)
// into a single value. AggCount is the default and ignores Event.Value.
type Aggregator string

const (
	AggCount Aggregator = "count"
	AggSum   Aggregator = "sum"
	AggAvg   Aggregator = "avg"
	AggMin   Aggregator = "min"
	AggMax   Aggregator = "max"
)

// Query is a typed pipeline against one Source. Bin is parsed via
// time.ParseDuration ("1h", "5m"); empty or zero means no time binning,
// returning one scalar Total per groupKey. The JSON shape is shared by
// the Wails bridge and the HTTP handler.
type Query struct {
	Source  string            `json:"source"`
	From    time.Time         `json:"from"`
	To      time.Time         `json:"to"`
	Filter  map[string]string `json:"filter,omitempty"`
	GroupBy []string          `json:"group_by,omitempty"`
	Bin     string            `json:"bin,omitempty"`
	Agg     Aggregator        `json:"agg,omitempty"`
	Limit   int               `json:"limit,omitempty"`
}

// Point is one (timestamp, value) sample on a Series, used when
// Query.Bin is non-empty.
type Point struct {
	Ts    time.Time `json:"ts"`
	Value float64   `json:"value"`
}

// Series is the per-group slice of the result. Key holds the
// distinguishing dim values; Points are populated when Query.Bin > 0,
// Total holds the scalar aggregate when Bin == 0.
type Series struct {
	Key    map[string]string `json:"key"`
	Points []Point           `json:"points,omitempty"`
	Total  float64           `json:"total,omitempty"`
}

// Result is the Run return value. Series ordering is stable across runs
// (sorted by joined Key string) so frontend tile colors don't flicker
// between refreshes.
type Result struct {
	Series []Series `json:"series"`
}

// SourceInfo describes one registered Source. Returned by
// Manager.ListSources for UI builders that render filter and group-by
// pickers without hard-coding dim names.
type SourceInfo struct {
	Name string   `json:"name"`
	Kind string   `json:"kind"`
	Dims []string `json:"dims"`
}
