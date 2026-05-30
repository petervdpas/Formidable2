package app

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
)

// fixtureSubsetLoader stands in for the storage-backed loader: it materializes
// records from an in-test map, and implements SubsetLoader so the planner seam
// loads just the narrowed ids (the same path the real adapter takes against
// form files).
type fixtureSubsetLoader struct{ by map[string]datacore.Record }

func (l *fixtureSubsetLoader) Records() ([]datacore.Record, error) {
	out := make([]datacore.Record, 0, len(l.by))
	for _, r := range l.by {
		out = append(out, r)
	}
	return out, nil
}

func (l *fixtureSubsetLoader) LoadSubset(ids []string) ([]datacore.Record, error) {
	out := make([]datacore.Record, 0, len(ids))
	for _, id := range ids {
		if r, ok := l.by[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// TestDatacorePlanner_NarrowsLikeInMemoryWhere is the planner-seam parity
// check: narrowing the record set through the real SQLite index must produce
// the same answer as selecting the same set in memory with Where over the full
// tensor. It proves the seam ("index narrows, datacore computes") is sound for
// both a facet predicate and a scalar-field equality predicate, and that the
// index actually narrowed (returned a strict subset, not everything).
func TestDatacorePlanner_NarrowsLikeInMemoryWhere(t *testing.T) {
	type form struct {
		id, region, status, tier string
	}
	fixture := []form{
		{"a.meta.json", "east", "active", "GOLD"},
		{"b.meta.json", "east", "active", "SILVER"},
		{"c.meta.json", "west", "retired", "GOLD"},
		{"d.meta.json", "west", "active", "GOLD"},
	}

	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	var forms []index.FormRow
	for _, f := range fixture {
		forms = append(forms, index.FormRow{
			Template: "basic.yaml", Filename: f.id, Mtime: 100,
			Facets: []index.FormFacet{{Key: "tier", Set: true, Selected: f.tier}},
			Values: []index.FormValueRow{
				{FieldKey: "region", ValueType: "text", Text: f.region},
				{FieldKey: "status", ValueType: "text", Text: f.status},
			},
		})
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	byID := map[string]datacore.Record{}
	full := datacore.New()
	for _, f := range fixture {
		rec := datacore.Record{
			ID:     f.id,
			Fields: map[string]string{"region": f.region, "status": f.status},
			Facets: map[string]string{"tier": f.tier},
		}
		byID[f.id] = rec
		full.Ingest(rec)
	}

	svc := datacore.NewServiceWithPlanner(
		func(string) datacore.Loader { return &fixtureSubsetLoader{by: byID} },
		newDatacoreIndexPlanner(idxM),
	)
	planner := newDatacoreIndexPlanner(idxM)

	// --- facet predicate: tier == GOLD ---
	facetPred := datacore.Predicate{Facets: map[string]string{"tier": "GOLD"}}
	ids, narrowed, err := planner.Plan("basic.yaml", facetPred)
	if err != nil || !narrowed {
		t.Fatalf("plan facet: narrowed=%v err=%v", narrowed, err)
	}
	if got := sortedCopy(ids); !equalStrings(got, []string{"a.meta.json", "c.meta.json", "d.meta.json"}) {
		t.Fatalf("facet narrow ids = %v, want the three GOLD forms", got)
	}
	gotFacet, err := svc.DistributionWhere("basic.yaml", "", facetPred, "region")
	if err != nil {
		t.Fatalf("DistributionWhere facet: %v", err)
	}
	wantFacet := full.View().Where("facet:tier", func(v string) bool { return v == "GOLD" }).Distribution("region")
	assertSameDistributionDC(t, "facet tier=GOLD", wantFacet, gotFacet)

	// --- scalar predicate: status == active ---
	scalarPred := datacore.Predicate{Equals: map[string]string{"status": "active"}}
	ids, narrowed, err = planner.Plan("basic.yaml", scalarPred)
	if err != nil || !narrowed {
		t.Fatalf("plan scalar: narrowed=%v err=%v", narrowed, err)
	}
	if len(ids) != 3 { // a, b, d are active
		t.Fatalf("scalar narrow ids = %v, want 3 active forms", sortedCopy(ids))
	}
	gotScalar, err := svc.DistributionWhere("basic.yaml", "", scalarPred, "region")
	if err != nil {
		t.Fatalf("DistributionWhere scalar: %v", err)
	}
	wantScalar := full.View().Where("status", func(v string) bool { return v == "active" }).Distribution("region")
	assertSameDistributionDC(t, "status=active", wantScalar, gotScalar)
}

// TestDatacorePlanner_UnhappyAndInteractions covers the paths that aren't the
// clean single-condition match: predicates that select nothing, an AND of two
// conditions with no overlap, and a real two-condition interaction. The seam
// must stay correct in every case (empty narrow -> empty tensor -> empty
// reduction, never an error or a silent full build).
func TestDatacorePlanner_UnhappyAndInteractions(t *testing.T) {
	type form struct{ id, region, status, tier string }
	fixture := []form{
		{"a.meta.json", "east", "active", "GOLD"},
		{"b.meta.json", "east", "active", "SILVER"},
		{"c.meta.json", "west", "retired", "GOLD"},
		{"d.meta.json", "west", "active", "GOLD"},
	}

	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	var forms []index.FormRow
	for _, f := range fixture {
		forms = append(forms, index.FormRow{
			Template: "basic.yaml", Filename: f.id, Mtime: 100,
			Facets: []index.FormFacet{{Key: "tier", Set: true, Selected: f.tier}},
			Values: []index.FormValueRow{
				{FieldKey: "region", ValueType: "text", Text: f.region},
				{FieldKey: "status", ValueType: "text", Text: f.status},
			},
		})
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	byID := map[string]datacore.Record{}
	full := datacore.New()
	for _, f := range fixture {
		rec := datacore.Record{
			ID:     f.id,
			Fields: map[string]string{"region": f.region, "status": f.status},
			Facets: map[string]string{"tier": f.tier},
		}
		byID[f.id] = rec
		full.Ingest(rec)
	}
	svc := datacore.NewServiceWithPlanner(
		func(string) datacore.Loader { return &fixtureSubsetLoader{by: byID} },
		newDatacoreIndexPlanner(idxM),
	)
	planner := newDatacoreIndexPlanner(idxM)

	// A facet value nobody carries: narrowed (the index answered) but empty.
	ids, narrowed, err := planner.Plan("basic.yaml", datacore.Predicate{Facets: map[string]string{"tier": "BRONZE"}})
	if err != nil || !narrowed || len(ids) != 0 {
		t.Fatalf("absent facet: ids=%v narrowed=%v err=%v, want narrowed empty", ids, narrowed, err)
	}
	got, err := svc.DistributionWhere("basic.yaml", "", datacore.Predicate{Facets: map[string]string{"tier": "BRONZE"}}, "region")
	if err != nil {
		t.Fatalf("DistributionWhere absent facet: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("absent-facet distribution = %v, want empty", got)
	}

	// A field nobody matches (and a field that doesn't exist at all): empty.
	for _, pred := range []datacore.Predicate{
		{Equals: map[string]string{"status": "archived"}},
		{Equals: map[string]string{"nosuchfield": "x"}},
	} {
		ids, narrowed, err = planner.Plan("basic.yaml", pred)
		if err != nil || !narrowed || len(ids) != 0 {
			t.Fatalf("no-match equals %+v: ids=%v narrowed=%v err=%v", pred, ids, narrowed, err)
		}
	}

	// AND with no overlap: tier=SILVER is only b, status=retired is only c.
	noOverlap := datacore.Predicate{
		Facets: map[string]string{"tier": "SILVER"},
		Equals: map[string]string{"status": "retired"},
	}
	ids, narrowed, err = planner.Plan("basic.yaml", noOverlap)
	if err != nil || !narrowed || len(ids) != 0 {
		t.Fatalf("no-overlap AND: ids=%v narrowed=%v err=%v, want empty", ids, narrowed, err)
	}

	// Real two-condition interaction: tier=GOLD AND status=active -> a, d.
	both := datacore.Predicate{
		Facets: map[string]string{"tier": "GOLD"},
		Equals: map[string]string{"status": "active"},
	}
	ids, narrowed, err = planner.Plan("basic.yaml", both)
	if err != nil || !narrowed {
		t.Fatalf("interaction plan: narrowed=%v err=%v", narrowed, err)
	}
	if !equalStrings(sortedCopy(ids), []string{"a.meta.json", "d.meta.json"}) {
		t.Fatalf("interaction ids = %v, want a,d", sortedCopy(ids))
	}
	gotBoth, err := svc.DistributionWhere("basic.yaml", "", both, "region")
	if err != nil {
		t.Fatalf("DistributionWhere interaction: %v", err)
	}
	wantBoth := full.View().
		Where("facet:tier", func(v string) bool { return v == "GOLD" }).
		Where("status", func(v string) bool { return v == "active" }).
		Distribution("region")
	assertSameDistributionDC(t, "tier=GOLD AND status=active", wantBoth, gotBoth)

	// No index wired: the planner declines, datacore falls back to a full build.
	nilPlanner := newDatacoreIndexPlanner(nil)
	if _, narrowed, err := nilPlanner.Plan("basic.yaml", both); err != nil || narrowed {
		t.Fatalf("nil-index plan: narrowed=%v err=%v, want declined", narrowed, err)
	}
}

// TestDatacorePlanner_ConcurrentNarrowedReads fires narrowed reductions from
// many goroutines at once. The planner reads the index (concurrent SQLite
// reads) and each call builds its own tensor, so nothing is shared mutably;
// under -race this proves the seam is safe to call from concurrent requests.
func TestDatacorePlanner_ConcurrentNarrowedReads(t *testing.T) {
	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	byID := map[string]datacore.Record{}
	var forms []index.FormRow
	for _, id := range []string{"a.meta.json", "b.meta.json", "c.meta.json", "d.meta.json"} {
		tier := "GOLD"
		if id == "b.meta.json" {
			tier = "SILVER"
		}
		forms = append(forms, index.FormRow{
			Template: "basic.yaml", Filename: id, Mtime: 100,
			Facets: []index.FormFacet{{Key: "tier", Set: true, Selected: tier}},
			Values: []index.FormValueRow{{FieldKey: "region", ValueType: "text", Text: "east"}},
		})
		byID[id] = datacore.Record{ID: id, Fields: map[string]string{"region": "east"}, Facets: map[string]string{"tier": tier}}
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	svc := datacore.NewServiceWithPlanner(
		func(string) datacore.Loader { return &fixtureSubsetLoader{by: byID} },
		newDatacoreIndexPlanner(idxM),
	)
	pred := datacore.Predicate{Facets: map[string]string{"tier": "GOLD"}}

	const workers = 16
	errs := make(chan error, workers)
	for range workers {
		go func() {
			d, err := svc.DistributionWhere("basic.yaml", "", pred, "region")
			if err != nil {
				errs <- err
				return
			}
			if len(d) != 1 || d[0].Value != "east" || d[0].Count != 3 {
				errs <- &countMismatch{d}
				return
			}
			errs <- nil
		}()
	}
	for range workers {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent narrowed read: %v", err)
		}
	}
}

type countMismatch struct{ got []datacore.Bucket }

func (e *countMismatch) Error() string { return "unexpected narrowed distribution" }

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertSameDistributionDC compares two datacore distributions by value.
func assertSameDistributionDC(t *testing.T, label string, want, got []datacore.Bucket) {
	t.Helper()
	w := map[string]int{}
	for _, b := range want {
		w[b.Value] = b.Count
	}
	g := map[string]int{}
	for _, b := range got {
		g[b.Value] = b.Count
	}
	if len(w) != len(g) {
		t.Fatalf("%s: bucket counts differ: want=%v got=%v", label, w, g)
	}
	for v, n := range w {
		if g[v] != n {
			t.Fatalf("%s: value %q want=%d got=%d", label, v, n, g[v])
		}
	}
}
