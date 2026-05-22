package monitor

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

// staticSource is a tiny in-memory Source for testing the Manager
// independent of any real backend (journal, log, request). Events are
// pre-baked; Events() filters by [from, to).
type staticSource struct {
	name   string
	kind   string
	dims   []string
	events []Event
}

func (s *staticSource) Name() string { return s.name }
func (s *staticSource) Kind() string { return s.kind }
func (s *staticSource) Dims() []string {
	return append([]string(nil), s.dims...)
}
func (s *staticSource) Events(from, to time.Time) []Event {
	out := make([]Event, 0, len(s.events))
	for _, e := range s.events {
		if !from.IsZero() && e.Ts.Before(from) {
			continue
		}
		if !to.IsZero() && !e.Ts.Before(to) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// fixedTime constructs a UTC time.Time at the given hour for synthetic
// data. Avoids time-zone churn in test fixtures.
func fixedTime(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, time.UTC)
}

// ─────────────────────────────────────────────────────────────────────
// Registry
// ─────────────────────────────────────────────────────────────────────

func TestRegister_RejectsNilAndEmptyName(t *testing.T) {
	m := NewManager()
	m.Register(nil)
	m.Register(&staticSource{name: ""})
	if got := len(m.ListSources()); got != 0 {
		t.Errorf("expected 0 registered, got %d", got)
	}
}

func TestRegister_PanicsOnDuplicateName(t *testing.T) {
	m := NewManager()
	m.Register(&staticSource{name: "x"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	m.Register(&staticSource{name: "x"})
}

func TestListSources_SortedByName(t *testing.T) {
	m := NewManager()
	m.Register(&staticSource{name: "zeta", kind: "z", dims: []string{"a"}})
	m.Register(&staticSource{name: "alpha", kind: "a", dims: []string{"b"}})
	m.Register(&staticSource{name: "mu", kind: "m"})
	got := m.ListSources()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Name != "alpha" || got[1].Name != "mu" || got[2].Name != "zeta" {
		t.Errorf("unsorted: %+v", got)
	}
	// Dims should be a defensive copy.
	got[0].Dims = append(got[0].Dims, "MUTATED")
	if d := m.ListSources()[0].Dims; len(d) != 1 || d[0] != "b" {
		t.Errorf("Dims not defensively copied: %v", d)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Run - error paths
// ─────────────────────────────────────────────────────────────────────

func TestRun_UnknownSourceErrors(t *testing.T) {
	m := NewManager()
	_, err := m.Run(Query{Source: "ghost"})
	if !errors.Is(err, ErrUnknownSource) {
		t.Errorf("expected ErrUnknownSource, got %v", err)
	}
}

func TestRun_FromAfterToErrors(t *testing.T) {
	m := NewManager()
	m.Register(&staticSource{name: "src"})
	_, err := m.Run(Query{
		Source: "src",
		From:   fixedTime(2026, 5, 9, 10),
		To:     fixedTime(2026, 5, 9, 8),
	})
	if err == nil {
		t.Error("expected error when from > to")
	}
}

func TestRun_BadBinErrors(t *testing.T) {
	m := NewManager()
	m.Register(&staticSource{name: "src"})
	if _, err := m.Run(Query{Source: "src", Bin: "1potato"}); err == nil {
		t.Error("expected parse-bin error")
	}
	if _, err := m.Run(Query{Source: "src", Bin: "-1h"}); err == nil {
		t.Error("expected negative-bin error")
	}
}

// ─────────────────────────────────────────────────────────────────────
// Run - happy paths (count, no bin)
// ─────────────────────────────────────────────────────────────────────

func journalLikeFixture() *staticSource {
	t10 := fixedTime(2026, 5, 9, 10)
	t11 := fixedTime(2026, 5, 9, 11)
	t12 := fixedTime(2026, 5, 9, 12)
	return &staticSource{
		name: "journal",
		kind: "mutation",
		dims: []string{"op", "template"},
		events: []Event{
			{Ts: t10, Kind: "mutation", Dims: map[string]string{"op": "create", "template": "recepten"}, Value: 1},
			{Ts: t10, Kind: "mutation", Dims: map[string]string{"op": "create", "template": "recepten"}, Value: 1},
			{Ts: t11, Kind: "mutation", Dims: map[string]string{"op": "update", "template": "recepten"}, Value: 1},
			{Ts: t11, Kind: "mutation", Dims: map[string]string{"op": "delete", "template": "recepten"}, Value: 1},
			{Ts: t12, Kind: "mutation", Dims: map[string]string{"op": "create", "template": "people"}, Value: 1},
		},
	}
}

func TestRun_NoGroupBy_NoBin_ReturnsScalarTotal(t *testing.T) {
	m := NewManager()
	m.Register(journalLikeFixture())
	res, err := m.Run(Query{Source: "journal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(res.Series))
	}
	if res.Series[0].Total != 5 {
		t.Errorf("total = %v, want 5", res.Series[0].Total)
	}
	if len(res.Series[0].Key) != 0 {
		t.Errorf("expected empty Key, got %+v", res.Series[0].Key)
	}
}

func TestRun_GroupByOp_NoBin(t *testing.T) {
	m := NewManager()
	m.Register(journalLikeFixture())
	res, err := m.Run(Query{Source: "journal", GroupBy: []string{"op"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"create": 3, "update": 1, "delete": 1}
	if len(res.Series) != len(want) {
		t.Fatalf("expected %d series, got %d (%+v)", len(want), len(res.Series), res.Series)
	}
	for _, s := range res.Series {
		op := s.Key["op"]
		if got := s.Total; got != want[op] {
			t.Errorf("op=%q total = %v, want %v", op, got, want[op])
		}
	}
}

func TestRun_FilterNarrowsEvents(t *testing.T) {
	m := NewManager()
	m.Register(journalLikeFixture())
	res, err := m.Run(Query{
		Source:  "journal",
		Filter:  map[string]string{"template": "recepten"},
		GroupBy: []string{"op"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"create": 2, "update": 1, "delete": 1}
	if len(res.Series) != len(want) {
		t.Fatalf("expected %d series, got %d", len(want), len(res.Series))
	}
	for _, s := range res.Series {
		if got := s.Total; got != want[s.Key["op"]] {
			t.Errorf("op=%q total = %v, want %v", s.Key["op"], got, want[s.Key["op"]])
		}
	}
}

func TestRun_FromToBoundsClipEvents(t *testing.T) {
	m := NewManager()
	m.Register(journalLikeFixture())
	// Only the 11:00 events should remain.
	res, err := m.Run(Query{
		Source: "journal",
		From:   fixedTime(2026, 5, 9, 11),
		To:     fixedTime(2026, 5, 9, 12),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Series[0].Total; got != 2 {
		t.Errorf("total in [11,12) = %v, want 2", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Run - binning
// ─────────────────────────────────────────────────────────────────────

func TestRun_HourlyBin_GroupByOp(t *testing.T) {
	m := NewManager()
	m.Register(journalLikeFixture())
	res, err := m.Run(Query{Source: "journal", GroupBy: []string{"op"}, Bin: "1h"})
	if err != nil {
		t.Fatal(err)
	}
	// op=create → 2 points (10:00→2, 12:00→1)
	// op=update → 1 point  (11:00→1)
	// op=delete → 1 point  (11:00→1)
	got := map[string][]Point{}
	for _, s := range res.Series {
		got[s.Key["op"]] = s.Points
	}
	if len(got["create"]) != 2 {
		t.Errorf("create points = %d, want 2 (%+v)", len(got["create"]), got["create"])
	}
	if v := got["create"][0].Value; v != 2 {
		t.Errorf("create[0].Value = %v, want 2", v)
	}
	if v := got["create"][1].Value; v != 1 {
		t.Errorf("create[1].Value = %v, want 1", v)
	}
	if !got["create"][0].Ts.Before(got["create"][1].Ts) {
		t.Errorf("points not sorted by ts")
	}
	for _, s := range res.Series {
		if s.Total != 0 {
			t.Errorf("Total should be 0 when binned, got %v for %+v", s.Total, s.Key)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// Run - aggregators other than count
// ─────────────────────────────────────────────────────────────────────

func valueSource() *staticSource {
	t1 := fixedTime(2026, 5, 9, 10)
	return &staticSource{
		name: "vals",
		kind: "log",
		events: []Event{
			{Ts: t1, Dims: map[string]string{"k": "a"}, Value: 5},
			{Ts: t1, Dims: map[string]string{"k": "a"}, Value: 7},
			{Ts: t1, Dims: map[string]string{"k": "a"}, Value: 3},
			{Ts: t1, Dims: map[string]string{"k": "b"}, Value: 100},
		},
	}
}

func TestRun_AggSum(t *testing.T) {
	m := NewManager()
	m.Register(valueSource())
	res, err := m.Run(Query{Source: "vals", GroupBy: []string{"k"}, Agg: AggSum})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"a": 15, "b": 100}
	for _, s := range res.Series {
		if s.Total != want[s.Key["k"]] {
			t.Errorf("k=%s sum = %v, want %v", s.Key["k"], s.Total, want[s.Key["k"]])
		}
	}
}

func TestRun_AggAvg(t *testing.T) {
	m := NewManager()
	m.Register(valueSource())
	res, err := m.Run(Query{Source: "vals", GroupBy: []string{"k"}, Agg: AggAvg})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Series {
		if s.Key["k"] == "a" && math.Abs(s.Total-5) > 1e-9 {
			t.Errorf("avg(a) = %v, want 5", s.Total)
		}
		if s.Key["k"] == "b" && math.Abs(s.Total-100) > 1e-9 {
			t.Errorf("avg(b) = %v, want 100", s.Total)
		}
	}
}

func TestRun_AggMinMax(t *testing.T) {
	m := NewManager()
	m.Register(valueSource())
	resMin, _ := m.Run(Query{Source: "vals", GroupBy: []string{"k"}, Agg: AggMin})
	resMax, _ := m.Run(Query{Source: "vals", GroupBy: []string{"k"}, Agg: AggMax})
	for _, s := range resMin.Series {
		if s.Key["k"] == "a" && s.Total != 3 {
			t.Errorf("min(a) = %v, want 3", s.Total)
		}
	}
	for _, s := range resMax.Series {
		if s.Key["k"] == "a" && s.Total != 7 {
			t.Errorf("max(a) = %v, want 7", s.Total)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// Run - limit + concurrency
// ─────────────────────────────────────────────────────────────────────

func TestRun_LimitCapsSeriesCount(t *testing.T) {
	m := NewManager()
	m.Register(journalLikeFixture())
	res, err := m.Run(Query{Source: "journal", GroupBy: []string{"op"}, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 2 {
		t.Errorf("limit=2 returned %d series", len(res.Series))
	}
}

func TestRun_ConcurrentSafe(t *testing.T) {
	m := NewManager()
	m.Register(journalLikeFixture())

	const goroutines = 8
	const opsEach = 50

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsEach; j++ {
				if _, err := m.Run(Query{Source: "journal", GroupBy: []string{"op"}}); err != nil {
					t.Errorf("Run: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// ─────────────────────────────────────────────────────────────────────
// Misc invariants
// ─────────────────────────────────────────────────────────────────────

func TestRun_EmptySourceReturnsNoSeries(t *testing.T) {
	m := NewManager()
	m.Register(&staticSource{name: "empty"})
	res, err := m.Run(Query{Source: "empty", GroupBy: []string{"op"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 0 {
		t.Errorf("empty source produced series: %+v", res.Series)
	}
}

func TestRun_SeriesOrderIsStable(t *testing.T) {
	m := NewManager()
	m.Register(journalLikeFixture())
	first, _ := m.Run(Query{Source: "journal", GroupBy: []string{"op"}})
	second, _ := m.Run(Query{Source: "journal", GroupBy: []string{"op"}})
	if len(first.Series) != len(second.Series) {
		t.Fatalf("len differs: %d vs %d", len(first.Series), len(second.Series))
	}
	for i := range first.Series {
		if first.Series[i].Key["op"] != second.Series[i].Key["op"] {
			t.Errorf("order differs at i=%d: %q vs %q", i, first.Series[i].Key["op"], second.Series[i].Key["op"])
		}
	}
}
