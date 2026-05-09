package monitor

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrUnknownSource is returned by Run when Query.Source doesn't match
// any registered Source.
var ErrUnknownSource = errors.New("monitor: unknown source")

// Manager owns the Source registry and runs Queries. Safe for
// concurrent use — registration and lookup are mutex-protected.
type Manager struct {
	mu      sync.RWMutex
	sources map[string]Source
}

// NewManager returns an empty manager. Register sources via Register
// before any Run call references them.
func NewManager() *Manager {
	return &Manager{sources: map[string]Source{}}
}

// Register adds a source to the registry. Panics on duplicate name —
// that's a composition-root bug and should fail loudly at startup,
// not silently shadow an earlier source.
func (m *Manager) Register(s Source) {
	if s == nil {
		return
	}
	name := s.Name()
	if name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[name]; ok {
		panic(fmt.Sprintf("monitor: source %q already registered", name))
	}
	m.sources[name] = s
}

// ListSources returns descriptors for every registered Source, sorted
// by Name. Used by the ListSources Wails/HTTP endpoint and by query
// builders that want to render dim pickers.
func (m *Manager) ListSources() []SourceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SourceInfo, 0, len(m.sources))
	for _, s := range m.sources {
		out = append(out, SourceInfo{
			Name: s.Name(),
			Kind: s.Kind(),
			Dims: append([]string(nil), s.Dims()...),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// bucketState accumulates the running stats for one (groupKey, bin)
// pair. min/max start at +Inf/-Inf so the first event populates both.
type bucketState struct {
	key   map[string]string
	count int
	sum   float64
	min   float64
	max   float64
}

// Run executes a Query against the matching Source. Failure modes:
// unknown source, malformed Bin string, From > To. Empty filter/groupBy
// is the "no constraint" case.
func (m *Manager) Run(q Query) (*Result, error) {
	m.mu.RLock()
	src, ok := m.sources[q.Source]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownSource, q.Source)
	}

	if !q.From.IsZero() && !q.To.IsZero() && q.From.After(q.To) {
		return nil, errors.New("monitor: from is after to")
	}

	binDur := time.Duration(0)
	if strings.TrimSpace(q.Bin) != "" {
		d, err := time.ParseDuration(q.Bin)
		if err != nil {
			return nil, fmt.Errorf("monitor: parse bin %q: %w", q.Bin, err)
		}
		if d < 0 {
			return nil, fmt.Errorf("monitor: negative bin %q", q.Bin)
		}
		binDur = d
	}

	agg := q.Agg
	if agg == "" {
		agg = AggCount
	}

	type bucketKey struct {
		groupKey string
		binStart int64 // unix nanos; zero when binDur == 0
	}
	buckets := map[bucketKey]*bucketState{}

	for _, ev := range src.Events(q.From, q.To) {
		if !matchesFilter(ev, q.Filter) {
			continue
		}
		gk := groupKeyFor(ev, q.GroupBy)
		bs := int64(0)
		if binDur > 0 {
			bs = ev.Ts.Truncate(binDur).UnixNano()
		}
		bk := bucketKey{groupKey: gk, binStart: bs}
		b, ok := buckets[bk]
		if !ok {
			b = &bucketState{
				key: keyMapFor(ev, q.GroupBy),
				min: math.Inf(1),
				max: math.Inf(-1),
			}
			buckets[bk] = b
		}
		b.count++
		b.sum += ev.Value
		if ev.Value < b.min {
			b.min = ev.Value
		}
		if ev.Value > b.max {
			b.max = ev.Value
		}
	}

	// Re-shape into Series per groupKey.
	seriesIdx := map[string]*Series{}
	seriesOrder := []string{}
	for bk, b := range buckets {
		s, ok := seriesIdx[bk.groupKey]
		if !ok {
			s = &Series{Key: b.key}
			seriesIdx[bk.groupKey] = s
			seriesOrder = append(seriesOrder, bk.groupKey)
		}
		v := aggregate(b, agg)
		if binDur > 0 {
			s.Points = append(s.Points, Point{Ts: time.Unix(0, bk.binStart).UTC(), Value: v})
		} else {
			s.Total += v
		}
	}

	sort.Strings(seriesOrder)
	out := make([]Series, 0, len(seriesOrder))
	for _, gk := range seriesOrder {
		s := seriesIdx[gk]
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Ts.Before(s.Points[j].Ts)
		})
		out = append(out, *s)
	}

	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}

	return &Result{Series: out}, nil
}

// matchesFilter returns true when every k=v in filter is present and
// equal in ev.Dims. Empty filter is "no constraint".
func matchesFilter(ev Event, filter map[string]string) bool {
	for k, want := range filter {
		got, ok := ev.Dims[k]
		if !ok || got != want {
			return false
		}
	}
	return true
}

// groupKeyFor builds a stable string key from the requested groupBy
// dimensions. Order-preserving so two events with the same dim values
// in the same groupBy order produce the same key.
func groupKeyFor(ev Event, groupBy []string) string {
	if len(groupBy) == 0 {
		return ""
	}
	parts := make([]string, 0, len(groupBy))
	for _, dim := range groupBy {
		parts = append(parts, dim+"="+ev.Dims[dim])
	}
	return strings.Join(parts, "\x1f")
}

// keyMapFor extracts the dim values that distinguish a Series.
func keyMapFor(ev Event, groupBy []string) map[string]string {
	out := make(map[string]string, len(groupBy))
	for _, dim := range groupBy {
		out[dim] = ev.Dims[dim]
	}
	return out
}

// aggregate reduces a bucket to a single value per the requested
// aggregator. AggCount ignores ev.Value entirely; the others operate
// on whatever Value the source supplied.
func aggregate(b *bucketState, agg Aggregator) float64 {
	switch agg {
	case AggSum:
		return b.sum
	case AggAvg:
		if b.count == 0 {
			return 0
		}
		return b.sum / float64(b.count)
	case AggMin:
		if b.count == 0 || math.IsInf(b.min, 1) {
			return 0
		}
		return b.min
	case AggMax:
		if b.count == 0 || math.IsInf(b.max, -1) {
			return 0
		}
		return b.max
	default: // AggCount
		return float64(b.count)
	}
}
