package datacore

import (
	"errors"
	"reflect"
	"testing"
)

// sliceLoader yields a fixed record set and counts how many records were
// materialized, so a test can prove narrowing ingested fewer than all.
type sliceLoader struct {
	recs   []Record
	loaded int
}

func (l *sliceLoader) Records() ([]Record, error) {
	l.loaded += len(l.recs)
	return l.recs, nil
}

// subsetLoader adds the SubsetLoader capability: it materializes only the named
// ids and records how many it loaded, so the test can assert the index path
// (not the full-scan fallback) ran.
type subsetLoader struct {
	by     map[string]Record
	loaded int
}

func (l *subsetLoader) Records() ([]Record, error) {
	out := make([]Record, 0, len(l.by))
	for _, r := range l.by {
		out = append(out, r)
	}
	l.loaded += len(out)
	return out, nil
}

func (l *subsetLoader) LoadSubset(ids []string) ([]Record, error) {
	out := make([]Record, 0, len(ids))
	for _, id := range ids {
		if r, ok := l.by[id]; ok {
			out = append(out, r)
			l.loaded++
		}
	}
	return out, nil
}

// fakePlanner returns a fixed id set, or declines (narrowed=false), or errors.
type fakePlanner struct {
	ids      []string
	narrowed bool
	err      error
	calls    int
}

func (p *fakePlanner) Plan(template string, pred Predicate) ([]string, bool, error) {
	p.calls++
	return p.ids, p.narrowed, p.err
}

func TestBuildNarrowed_SubsetLoaderIngestsOnlyMatches(t *testing.T) {
	loader := &subsetLoader{by: map[string]Record{}}
	for _, r := range sampleRecords() {
		loader.by[r.ID] = r
	}
	planner := &fakePlanner{ids: []string{"a", "c"}, narrowed: true}

	tn, err := buildNarrowed(loader, planner, "t", Predicate{Facets: map[string]string{"status": "active"}})
	if err != nil {
		t.Fatalf("buildNarrowed: %v", err)
	}
	if got := tn.View().Count(); got != 2 {
		t.Fatalf("narrowed count = %d, want 2", got)
	}
	if planner.calls != 1 {
		t.Fatalf("planner calls = %d, want 1", planner.calls)
	}
	if loader.loaded != 2 {
		t.Fatalf("loaded %d records, want only the 2 narrowed ones", loader.loaded)
	}
}

func TestBuildNarrowed_FallsBackToFullScanWithoutSubsetLoader(t *testing.T) {
	loader := &sliceLoader{recs: sampleRecords()}
	planner := &fakePlanner{ids: []string{"a", "c"}, narrowed: true}

	tn, err := buildNarrowed(loader, planner, "t", Predicate{Facets: map[string]string{"status": "active"}})
	if err != nil {
		t.Fatalf("buildNarrowed: %v", err)
	}
	// Correct answer even though the plain loader had to read all then filter.
	if got := tn.View().Count(); got != 2 {
		t.Fatalf("fallback narrowed count = %d, want 2", got)
	}
}

func TestBuildNarrowed_EmptyPredicateBuildsAll(t *testing.T) {
	loader := &sliceLoader{recs: sampleRecords()}
	planner := &fakePlanner{ids: []string{"a"}, narrowed: true}

	tn, err := buildNarrowed(loader, planner, "t", Predicate{})
	if err != nil {
		t.Fatalf("buildNarrowed: %v", err)
	}
	if got := tn.View().Count(); got != 3 {
		t.Fatalf("empty-predicate count = %d, want 3 (all)", got)
	}
	if planner.calls != 0 {
		t.Fatalf("planner consulted %d times for an empty predicate, want 0", planner.calls)
	}
}

func TestBuildNarrowed_NilPlannerBuildsAll(t *testing.T) {
	loader := &sliceLoader{recs: sampleRecords()}

	tn, err := buildNarrowed(loader, nil, "t", Predicate{Facets: map[string]string{"status": "active"}})
	if err != nil {
		t.Fatalf("buildNarrowed: %v", err)
	}
	if got := tn.View().Count(); got != 3 {
		t.Fatalf("nil-planner count = %d, want 3 (all)", got)
	}
}

func TestBuildNarrowed_PlannerDeclinesBuildsAll(t *testing.T) {
	loader := &sliceLoader{recs: sampleRecords()}
	planner := &fakePlanner{narrowed: false}

	tn, err := buildNarrowed(loader, planner, "t", Predicate{Search: "anything"})
	if err != nil {
		t.Fatalf("buildNarrowed: %v", err)
	}
	if got := tn.View().Count(); got != 3 {
		t.Fatalf("declined-narrow count = %d, want 3 (all)", got)
	}
}

func TestBuildNarrowed_PlannerErrorPropagates(t *testing.T) {
	loader := &sliceLoader{recs: sampleRecords()}
	boom := errors.New("index unavailable")
	planner := &fakePlanner{err: boom, narrowed: true}

	if _, err := buildNarrowed(loader, planner, "t", Predicate{Search: "x"}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// A non-SubsetLoader falls back to reading every record then filtering. When
// that full read errors, loadSubset (and so buildNarrowed) propagates it.
func TestBuildNarrowed_FallbackLoaderErrorPropagates(t *testing.T) {
	boom := errors.New("full scan failed")
	planner := &fakePlanner{ids: []string{"a"}, narrowed: true}

	if _, err := buildNarrowed(failLoader{err: boom}, planner, "t", Predicate{Search: "x"}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// The seam contract: narrowing via the planner equals selecting the same set
// in memory with Where over the full tensor. Here the predicate "status ==
// active" must match an in-memory Where on the facet:status cell, then the
// downstream reduction (team distribution) must agree.
func TestBuildNarrowed_AgreesWithInMemoryWhere(t *testing.T) {
	recs := sampleRecords()
	loader := &subsetLoader{by: map[string]Record{}}
	full := New()
	for _, r := range recs {
		loader.by[r.ID] = r
		full.Ingest(r)
	}
	planner := &fakePlanner{ids: []string{"a", "c"}, narrowed: true}

	narrowed, err := buildNarrowed(loader, planner, "t", Predicate{Facets: map[string]string{"status": "active"}})
	if err != nil {
		t.Fatalf("buildNarrowed: %v", err)
	}
	got := narrowed.View().Distribution("team")
	want := full.View().Where("facet:status", func(v string) bool { return v == "active" }).Distribution("team")

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("narrowed distribution = %+v, want in-memory Where %+v", got, want)
	}
}
