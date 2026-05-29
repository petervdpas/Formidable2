package app

import (
	"math"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
)

// TestDatacore_DistributionMatchesIndex proves the new datacore tensor and
// the trusted index path produce the same distribution on the same data, for
// both a scalar field and a table column. The index (SQLite EAV) is what stat
// reads today; this is the parity check that lets datacore grow toward
// replacing that path without guessing whether the numbers agree.
//
// The fixture is defined once and projected into both representations, so the
// two engines genuinely see identical input:
//   - index: form_values rows (scalar status, one per form; table "items"
//     column 0 "kind", one row per cell)
//   - datacore: a Record per form (status field; "items" table whose rows
//     each become their own identity, reached by Follow)
func TestDatacore_DistributionMatchesIndex(t *testing.T) {
	type form struct {
		id     string
		status string
		kinds  []string // one table row per kind
	}
	fixture := []form{
		{id: "a.meta.json", status: "high", kinds: []string{"hw", "sw"}},
		{id: "b.meta.json", status: "low", kinds: []string{"hw"}},
		{id: "c.meta.json", status: "high", kinds: []string{"hw", "hw"}},
	}

	// --- index path ---
	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	col0 := 0
	var forms []index.FormRow
	for _, f := range fixture {
		vals := []index.FormValueRow{{FieldKey: "status", ValueType: "text", Text: f.status}}
		for _, k := range f.kinds {
			c := col0
			vals = append(vals, index.FormValueRow{FieldKey: "items", Col: &c, ValueType: "text", Text: k})
		}
		forms = append(forms, index.FormRow{
			Template: "basic.yaml", Filename: f.id, Mtime: 100, Values: vals,
		})
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	idxStatus, err := idxM.ValueDistribution("basic.yaml", "status", nil)
	if err != nil {
		t.Fatalf("index status distribution: %v", err)
	}
	idxKind, err := idxM.ValueDistribution("basic.yaml", "items", &col0)
	if err != nil {
		t.Fatalf("index kind distribution: %v", err)
	}

	// --- datacore path ---
	dt := datacore.New()
	for _, f := range fixture {
		rows := make([]map[string]string, len(f.kinds))
		for i, k := range f.kinds {
			rows[i] = map[string]string{"kind": k}
		}
		dt.Ingest(datacore.Record{
			ID:     f.id,
			Fields: map[string]string{"status": f.status},
			Tables: map[string][]map[string]string{"items": rows},
		})
	}
	dcStatus := dt.View().Distribution("status")
	dcKind := dt.View().Follow("items").Distribution("kind")

	assertSameDistribution(t, "status (scalar field)", idxStatus, dcStatus)
	assertSameDistribution(t, "kind (table column)", idxKind, dcKind)
}

// TestDatacore_CrossMatchesIndexFacetCross proves the rank-2 cross-tab agrees
// with index.FacetCross on the same data. Facets ingest into datacore as
// "facet:<key>" fields, so crossing two facet fields must reproduce the
// facet-cross the index computes by joining form_facets to itself. Forms set
// both facets with non-empty selections, so no blank category diverges
// between index (COALESCE '') and datacore (blank skipped).
func TestDatacore_CrossMatchesIndexFacetCross(t *testing.T) {
	type form struct {
		id, prio, stage string
	}
	fixture := []form{
		{"a.meta.json", "high", "draft"},
		{"b.meta.json", "high", "draft"},
		{"c.meta.json", "high", "live"},
		{"d.meta.json", "low", "live"},
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
			Facets: []index.FormFacet{
				{Key: "prio", Set: true, Selected: f.prio},
				{Key: "stage", Set: true, Selected: f.stage},
			},
		})
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	idxCross, err := idxM.FacetCross("basic.yaml", "prio", "stage")
	if err != nil {
		t.Fatalf("index facet cross: %v", err)
	}

	dt := datacore.New()
	for _, f := range fixture {
		dt.Ingest(datacore.Record{
			ID:     f.id,
			Facets: map[string]string{"prio": f.prio, "stage": f.stage},
		})
	}
	dcCross := dt.View().Cross("facet:prio", "facet:stage")

	want := map[[2]string]int{}
	for _, c := range idxCross {
		want[[2]string{c.A, c.B}] = c.Count
	}
	got := map[[2]string]int{}
	for _, c := range dcCross.Cells {
		got[[2]string{c.Row, c.Col}] = c.Count
	}
	if len(want) != len(got) {
		t.Fatalf("cross cell counts differ: index=%v datacore=%v", want, got)
	}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("cell %v: index=%d datacore=%d", k, n, got[k])
		}
	}
}

// TestDatacore_AggregateMatchesIndexNumericValues proves the numeric
// reduction agrees with the index's numeric extraction. index.NumericValues
// returns the raw coercible numbers stat reduces; datacore.Aggregate reduces
// the same field directly. Sum, min, and max over the same values must match,
// for a scalar field and a table column reached by Follow.
func TestDatacore_AggregateMatchesIndexNumericValues(t *testing.T) {
	type form struct {
		id     string
		amount float64
		costs  []float64
	}
	fixture := []form{
		{"a.meta.json", 10, []float64{100, 50}},
		{"b.meta.json", 30, []float64{25}},
		{"c.meta.json", 20, []float64{200, 5}},
	}

	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	col0 := 0
	var forms []index.FormRow
	for _, f := range fixture {
		amt := f.amount
		vals := []index.FormValueRow{{FieldKey: "amount", ValueType: "number", Num: &amt, Text: ftoa(amt)}}
		for _, c := range f.costs {
			cc, ci := c, col0
			vals = append(vals, index.FormValueRow{FieldKey: "items", Col: &ci, ValueType: "number", Num: &cc, Text: ftoa(c)})
		}
		forms = append(forms, index.FormRow{Template: "basic.yaml", Filename: f.id, Mtime: 100, Values: vals})
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	dt := datacore.New()
	for _, f := range fixture {
		rows := make([]map[string]string, len(f.costs))
		for i, c := range f.costs {
			rows[i] = map[string]string{"cost": ftoa(c)}
		}
		dt.Ingest(datacore.Record{
			ID:     f.id,
			Fields: map[string]string{"amount": ftoa(f.amount)},
			Tables: map[string][]map[string]string{"items": rows},
		})
	}

	assertSameAgg(t, "amount (scalar)", idxM, "amount", nil, dt.View().Aggregate("amount"))
	assertSameAgg(t, "cost (table column)", idxM, "items", &col0, dt.View().Follow("items").Aggregate("cost"))
}

func assertSameAgg(t *testing.T, label string, idxM *index.Manager, field string, col *int, dc datacore.Aggregate) {
	t.Helper()
	vals, err := idxM.NumericValues("basic.yaml", field, col)
	if err != nil {
		t.Fatalf("%s: index NumericValues: %v", label, err)
	}
	var sum, min, max float64
	for i, v := range vals {
		sum += v
		if i == 0 || v < min {
			min = v
		}
		if i == 0 || v > max {
			max = v
		}
	}
	if dc.N != len(vals) {
		t.Fatalf("%s: N index=%d datacore=%d", label, len(vals), dc.N)
	}
	if math.Abs(dc.Sum-sum) > 1e-9 || math.Abs(dc.Min-min) > 1e-9 || math.Abs(dc.Max-max) > 1e-9 {
		t.Fatalf("%s: index sum=%g min=%g max=%g, datacore sum=%g min=%g max=%g",
			label, sum, min, max, dc.Sum, dc.Min, dc.Max)
	}
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func assertSameDistribution(t *testing.T, label string, idx []index.Bucket, dc []datacore.Bucket) {
	t.Helper()
	want := map[string]int{}
	for _, b := range idx {
		want[b.Label] = b.Count
	}
	got := map[string]int{}
	for _, b := range dc {
		got[b.Value] = b.Count
	}
	if len(want) != len(got) {
		t.Fatalf("%s: bucket counts differ: index=%v datacore=%v", label, want, got)
	}
	for v, n := range want {
		if got[v] != n {
			t.Fatalf("%s: value %q index=%d datacore=%d", label, v, n, got[v])
		}
	}
}
