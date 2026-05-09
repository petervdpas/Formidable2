// Package monitor is the generic observation surface over Formidable's
// internal event streams. One Event shape, multiple Source plug-ins
// (journal, app log, request log) — the Manager runs typed Query
// values against any registered source and returns a typed Result.
//
// Design:
//   - Sources project their native records into Event{Ts, Kind, Dims, Value}.
//   - Query{Filter, GroupBy, Bin, Agg} is a typed pipeline. No DSL.
//   - Result{Series, Series.Points or Series.Total} feeds chart components
//     directly and round-trips cleanly across the Wails bridge.
//
// New sources slot in by implementing Source — no Manager surgery, no
// new Wails methods. The HTTP handler at /api/monitor/* exposes the
// same Query shape as JSON for external consumers.
package monitor

import "time"

// Event is the canonical observation record. Sources project whatever
// they see (a journal entry, a log line, an HTTP request) into this
// shape. Dimensions are free-form key/value strings; consumers filter
// and group by dim names. Value is the per-event numeric contribution
// to non-count aggregations (default 1.0).
type Event struct {
	Ts    time.Time
	Kind  string
	Dims  map[string]string
	Value float64
}

// Source is a stream of Events bounded by a time range. Implementations
// must respect [from, to] inclusive-exclusive (event.Ts >= from,
// event.Ts < to). Empty time range (zero from, zero to) means "all".
type Source interface {
	// Name is the Source identifier the Query.Source field selects
	// against. Stable; chosen at registration time.
	Name() string

	// Kind is a coarse label for the Source's event domain — used
	// only by ListSources for UI groupings ("journal", "log", "request").
	Kind() string

	// Dims is the set of dimension names this Source emits. Informs
	// query-builder UIs and the SourceInfo response. Order is stable.
	Dims() []string

	// Events yields events in [from, to). Implementations may stream
	// from disk; callers must consume promptly.
	Events(from, to time.Time) []Event
}

// Aggregator is how the Manager reduces multiple Events with the same
// (groupKey, binStart) into a single value. AggCount is the default
// and ignores Event.Value entirely.
type Aggregator string

const (
	AggCount Aggregator = "count"
	AggSum   Aggregator = "sum"
	AggAvg   Aggregator = "avg"
	AggMin   Aggregator = "min"
	AggMax   Aggregator = "max"
)

// Query is a typed pipeline against one Source. Bin is parsed via
// time.ParseDuration ("1h", "5m", "24h") — empty/zero means "no time
// binning, return one scalar Total per groupKey." JSON shape is used
// by both the Wails bridge and the HTTP handler.
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

// Point is one (timestamp, value) sample on a Series. Used when
// Query.Bin is non-empty.
type Point struct {
	Ts    time.Time `json:"ts"`
	Value float64   `json:"value"`
}

// Series is the per-group slice of the result. Key holds the dim
// values that distinguish this Series; Points are populated when
// Query.Bin > 0; Total holds the scalar aggregate when Bin == 0.
type Series struct {
	Key    map[string]string `json:"key"`
	Points []Point           `json:"points,omitempty"`
	Total  float64           `json:"total,omitempty"`
}

// Result is the Run() return value. Series ordering is stable across
// runs (sorted by joined Key string) so frontend tile colors don't
// flicker between refreshes.
type Result struct {
	Series []Series `json:"series"`
}

// SourceInfo describes one registered Source. Returned by
// Manager.ListSources() for UI builders that want to render filter
// and group-by pickers without hard-coding dim names.
type SourceInfo struct {
	Name string   `json:"name"`
	Kind string   `json:"kind"`
	Dims []string `json:"dims"`
}
