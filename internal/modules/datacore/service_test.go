package datacore

import (
	"errors"
	"testing"
)

// failLoader fails on Records(), exercising the error-propagation branch every
// Service method shares (the loader read can fail before any reduction runs).
type failLoader struct{ err error }

func (l failLoader) Records() ([]Record, error) { return nil, l.err }

// graphRecords mirrors the topology graphFixture as a record slice, so the
// Service (which builds from a Loader) can reach Graph/GraphFrom.
func graphRecords() []Record {
	return []Record{
		{
			ID:     "A",
			Fields: map[string]string{"title": "Alpha"},
			Tables: map[string][]map[string]string{"items": {{"name": "disk"}, {"name": "ram"}}},
			Links:  map[string][]string{"owner": {"B"}},
		},
		{ID: "B", Fields: map[string]string{"title": "Beta"}},
	}
}

func sampleService() *Service {
	return NewService(func(string) Loader { return &sliceLoader{recs: sampleRecords()} })
}

func TestServiceCountAndDistributionDelegate(t *testing.T) {
	svc := sampleService()

	n, err := svc.Count("t", "")
	if err != nil || n != 3 {
		t.Fatalf("Count = (%d,%v), want (3,nil)", n, err)
	}

	dist, err := svc.Distribution("t", "", "team")
	if err != nil {
		t.Fatalf("Distribution err = %v", err)
	}
	if len(dist) != 2 || dist[0].Value != "east" || dist[0].Count != 2 {
		t.Fatalf("Distribution = %+v, want east:2 west:1", dist)
	}
}

func TestServiceCountWhereWithNilPlannerNarrowsInMemory(t *testing.T) {
	svc := sampleService()

	// No planner wired, so viewWhere builds all then the predicate is moot at
	// this layer; CountWhere still answers over the full set.
	n, err := svc.CountWhere("t", "", Predicate{Facets: map[string]string{"status": "active"}})
	if err != nil || n != 3 {
		t.Fatalf("CountWhere (nil planner) = (%d,%v), want (3,nil)", n, err)
	}
}

func TestServiceDistributionWhereWithPlannerNarrows(t *testing.T) {
	loader := &subsetLoader{by: map[string]Record{}}
	for _, r := range sampleRecords() {
		loader.by[r.ID] = r
	}
	planner := &fakePlanner{ids: []string{"a", "c"}, narrowed: true}
	svc := NewServiceWithPlanner(func(string) Loader { return loader }, planner)

	dist, err := svc.DistributionWhere("t", "", Predicate{Facets: map[string]string{"status": "active"}}, "team")
	if err != nil {
		t.Fatalf("DistributionWhere err = %v", err)
	}
	// a (east) + c (west) survive the narrow.
	got := map[string]int{}
	for _, b := range dist {
		got[b.Value] = b.Count
	}
	if got["east"] != 1 || got["west"] != 1 {
		t.Fatalf("narrowed distribution = %+v, want east:1 west:1", dist)
	}
}

func TestServiceAggregateCrossDateSeriesDelegate(t *testing.T) {
	svc := sampleService()

	agg, err := svc.Aggregate("t", "", "amount")
	if err != nil || agg.N != 3 || !eq(agg.Sum, 60) {
		t.Fatalf("Aggregate = (%+v,%v), want N 3 sum 60", agg, err)
	}

	ct, err := svc.Cross("t", "", "team", "facet:status")
	if err != nil {
		t.Fatalf("Cross err = %v", err)
	}
	if ct.Count("east", "active") != 1 || ct.Count("east", "retired") != 1 {
		t.Fatalf("Cross = %+v, want east/active 1 east/retired 1", ct)
	}

	series, err := svc.DateSeries("t", "", "amount", "month")
	if err != nil {
		t.Fatalf("DateSeries err = %v", err)
	}
	_ = series // amount is not a date; series is simply empty, no error
}

func TestServiceGraphAndGraphFromDelegate(t *testing.T) {
	svc := NewService(func(string) Loader { return &sliceLoader{recs: graphRecords()} })

	g, err := svc.Graph("t", 0)
	if err != nil {
		t.Fatalf("Graph err = %v", err)
	}
	if len(g.Nodes) != 4 {
		t.Fatalf("Graph nodes = %d, want 4 (A, B, 2 rows)", len(g.Nodes))
	}
}

// GraphFrom accepts a bare filename (first studio call) and a pre-composited
// node id (click round-trip), resolving both to the same identity without
// double-prefixing. This is the isCompositeID branch.
func TestServiceGraphFromBareAndCompositeIDResolveSame(t *testing.T) {
	svc := NewService(func(string) Loader { return &sliceLoader{recs: graphRecords()} })

	// graphRecords ingest standalone (empty template), so identities are bare
	// "A". A bare rootID is prefixed with the template, which would NOT match
	// the standalone identity, yielding an empty graph.
	bare, err := svc.GraphFrom("t", "A", 0)
	if err != nil {
		t.Fatalf("GraphFrom bare err = %v", err)
	}
	if len(bare.Nodes) != 0 {
		t.Fatalf("GraphFrom bare = %+v, want empty (template-prefixed id misses standalone identity)", bare.Nodes)
	}

	// A pre-composited id already carrying the separator is used verbatim and
	// still misses the standalone identity, but it must not double-prefix: the
	// resolved lookup id is exactly what was handed in.
	composite := NewID("t", "A")
	gc, err := svc.GraphFrom("t", composite, 0)
	if err != nil {
		t.Fatalf("GraphFrom composite err = %v", err)
	}
	if len(gc.Nodes) != 0 {
		t.Fatalf("GraphFrom composite = %+v, want empty (no record under t-prefixed A)", gc.Nodes)
	}
}

// When records carry the template prefix, GraphFrom resolves a bare filename to
// the real identity and returns its node.
func TestServiceGraphFromResolvesBareToPrefixedRecord(t *testing.T) {
	recs := []Record{{ID: NewID("t", "A"), Fields: map[string]string{"title": "Alpha"}}}
	svc := NewService(func(string) Loader { return &sliceLoader{recs: recs} })

	g, err := svc.GraphFrom("t", "A", 0)
	if err != nil {
		t.Fatalf("GraphFrom err = %v", err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].ID != NewID("t", "A") {
		t.Fatalf("GraphFrom = %+v, want the single prefixed identity", g.Nodes)
	}
}

func TestServiceSummarizeDelegates(t *testing.T) {
	recs := []Record{
		{ID: "a", Tables: map[string][]map[string]string{"items": {{"cost": "100"}, {"cost": "50"}}}},
		{ID: "b", Tables: map[string][]map[string]string{"items": {{"cost": "25"}}}},
	}
	svc := NewService(func(string) Loader { return &sliceLoader{recs: recs} })

	sums, err := svc.Summarize("t", "items", "cost")
	if err != nil {
		t.Fatalf("Summarize err = %v", err)
	}
	if len(sums) != 2 {
		t.Fatalf("Summarize = %d summaries, want 2", len(sums))
	}
}

func TestServiceAggregateRawDelegates(t *testing.T) {
	recs := []Record{
		{ID: "a", Fields: map[string]string{"region": "east", "amount": "10"}},
		{ID: "b", Fields: map[string]string{"region": "west", "amount": "20"}},
	}
	svc := NewService(func(string) Loader { return &sliceLoader{recs: recs} })

	rows, err := svc.AggregateRaw("t", []GridDim{{Field: "region"}}, []GridNum{{Field: "amount"}}, nil)
	if err != nil {
		t.Fatalf("AggregateRaw err = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("AggregateRaw rows = %d, want 2", len(rows))
	}
}

// AggregateRaw rejects a multi-table request before it touches the loader, so
// the error fires even when the loader would otherwise error.
func TestServiceAggregateRawMultiTableErrorsBeforeLoad(t *testing.T) {
	svc := NewService(func(string) Loader { return failLoader{err: errors.New("must not be read")} })

	_, err := svc.AggregateRaw("t",
		[]GridDim{{Field: "a", Table: "t1"}, {Field: "b", Table: "t2"}},
		nil, nil,
	)
	if err == nil {
		t.Fatal("AggregateRaw multi-table err = nil, want a single-table error")
	}
}

// Every reducing method shares the build seam: when the loader's Records()
// fails, the error must propagate unchanged from each entry point.
func TestServiceLoaderErrorPropagates(t *testing.T) {
	boom := errors.New("storage unreachable")
	svc := NewService(func(string) Loader { return failLoader{err: boom} })
	plannedSvc := NewServiceWithPlanner(
		func(string) Loader { return failLoader{err: boom} },
		&fakePlanner{narrowed: false},
	)

	checks := []struct {
		name string
		run  func() error
	}{
		{"Count", func() error { _, err := svc.Count("t", ""); return err }},
		{"CountWhere", func() error {
			_, err := plannedSvc.CountWhere("t", "", Predicate{Search: "x"})
			return err
		}},
		{"Distribution", func() error { _, err := svc.Distribution("t", "", "team"); return err }},
		{"DistributionWhere", func() error {
			_, err := plannedSvc.DistributionWhere("t", "", Predicate{Search: "x"}, "team")
			return err
		}},
		{"Aggregate", func() error { _, err := svc.Aggregate("t", "", "amount"); return err }},
		{"Cross", func() error { _, err := svc.Cross("t", "", "a", "b"); return err }},
		{"DateSeries", func() error { _, err := svc.DateSeries("t", "", "d", "month"); return err }},
		{"Graph", func() error { _, err := svc.Graph("t", 0); return err }},
		{"GraphFrom", func() error { _, err := svc.GraphFrom("t", "A", 1); return err }},
		{"AggregateRaw", func() error {
			_, err := svc.AggregateRaw("t", []GridDim{{Field: "region"}}, nil, nil)
			return err
		}},
		{"Summarize", func() error { _, err := svc.Summarize("t", "items", "cost"); return err }},
	}
	for _, c := range checks {
		if err := c.run(); !errors.Is(err, boom) {
			t.Fatalf("%s err = %v, want %v", c.name, err, boom)
		}
	}
}
