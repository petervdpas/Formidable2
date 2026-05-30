package app

import (
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
)

// Layer 1 of the stat-on-datacore parity gate (design/datacore-stat-migration.md):
// every stat.Index method, the datacore-backed adapter vs the trusted index, on
// one shared fixture, plus the unhappy and boundary cases. Both engines are fed
// from the same logical forms so any divergence is the engine, not the input.

// statForm is one logical form projected into both an index FormRow and a
// datacore Record. numeric/date typing is declared so the index side stores the
// right column (num_value vs text_value); the datacore side only ever sees the
// string and re-types on demand.
type statForm struct {
	id     string
	text   map[string]string // text field -> value
	num    map[string]string // numeric field -> value ("" = absent; "oops" = anomaly)
	date   map[string]string // date field -> ISO value
	facets map[string]string // facet -> selected option
	costs  []string          // "items" table, one "cost" column per row
}

func newDatacoreStatAdapter(t *testing.T, forms []statForm) (*datacoreStatIndex, *index.Manager) {
	t.Helper()

	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	col0 := 0
	var rows []index.FormRow
	var recs []datacore.Record
	for _, f := range forms {
		var vals []index.FormValueRow
		fields := map[string]string{}
		for k, v := range f.text {
			vals = append(vals, index.FormValueRow{FieldKey: k, ValueType: "text", Text: v})
			fields[k] = v
		}
		for k, v := range f.num {
			if v == "" {
				continue
			}
			row := index.FormValueRow{FieldKey: k, ValueType: "number", Text: v}
			if n, err := parseFloat(v); err == nil {
				row.Num = &n
			}
			vals = append(vals, row)
			fields[k] = v
		}
		for k, v := range f.date {
			vals = append(vals, index.FormValueRow{FieldKey: k, ValueType: "date", Text: v})
			fields[k] = v
		}
		var costRows []map[string]string
		for _, c := range f.costs {
			cc, ci := mustFloat(t, c), col0
			vals = append(vals, index.FormValueRow{FieldKey: "items", Col: &ci, ValueType: "number", Num: &cc, Text: c})
			costRows = append(costRows, map[string]string{"cost": c})
		}
		var facets []index.FormFacet
		for k, v := range f.facets {
			facets = append(facets, index.FormFacet{Key: k, Set: true, Selected: v})
		}
		rows = append(rows, index.FormRow{Template: "basic.yaml", Filename: f.id, Mtime: 100, Values: vals, Facets: facets})

		rec := datacore.Record{ID: f.id, Fields: fields, Facets: f.facets}
		if len(costRows) > 0 {
			rec.Tables = map[string][]map[string]string{"items": costRows}
		}
		recs = append(recs, rec)
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     rows,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	dc := datacore.NewService(func(string) datacore.Loader { return dcRecordLoader{recs} })
	cols := fakeColumnNamer{"items": {0: "cost"}}
	return newDatacoreStatIndex(dc, cols), idxM
}

type dcRecordLoader struct{ recs []datacore.Record }

func (l dcRecordLoader) Records() ([]datacore.Record, error) { return l.recs, nil }

type fakeColumnNamer map[string]map[int]string

func (n fakeColumnNamer) ColumnKey(_, fieldKey string, col int) (string, bool) {
	if m, ok := n[fieldKey]; ok {
		if k, ok := m[col]; ok {
			return k, true
		}
	}
	return "", false
}

func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

func richStatFixture() []statForm {
	return []statForm{
		{id: "a.meta.json", text: map[string]string{"status": "high"}, num: map[string]string{"amount": "10"}, date: map[string]string{"due": "2026-01-15"}, facets: map[string]string{"tier": "GOLD", "stage": "draft"}, costs: []string{"100", "50"}},
		{id: "b.meta.json", text: map[string]string{"status": "low"}, num: map[string]string{"amount": "30"}, date: map[string]string{"due": "2026-01-20"}, facets: map[string]string{"tier": "SILVER", "stage": "draft"}, costs: []string{"25"}},
		{id: "c.meta.json", text: map[string]string{"status": "high"}, num: map[string]string{"amount": "oops"}, date: map[string]string{"due": "2026-02-05"}, facets: map[string]string{"tier": "GOLD", "stage": "live"}, costs: []string{"200", "5"}},
		{id: "d.meta.json", text: map[string]string{"status": "high"}, num: map[string]string{"amount": ""}, date: map[string]string{"due": "2027-03-01"}, facets: map[string]string{"tier": "GOLD", "stage": "live"}},
	}
}

func TestStatAdapter_TotalFormsParity(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())
	wantTotal, err := idxM.TotalForms("basic.yaml")
	if err != nil {
		t.Fatalf("index TotalForms: %v", err)
	}
	got, err := a.TotalForms("basic.yaml")
	if err != nil {
		t.Fatalf("adapter TotalForms: %v", err)
	}
	if got != wantTotal {
		t.Fatalf("TotalForms = %d, want %d", got, wantTotal)
	}
}

func TestStatAdapter_ValueDistributionParity(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())

	// scalar text field
	idxD, _ := idxM.ValueDistribution("basic.yaml", "status", nil)
	dcD, err := a.ValueDistribution("basic.yaml", "status", nil)
	if err != nil {
		t.Fatalf("adapter ValueDistribution: %v", err)
	}
	assertBucketsEqual(t, "status", idxD, dcD)

	// table column (cost), reached by col 0
	col0 := 0
	idxC, _ := idxM.ValueDistribution("basic.yaml", "items", &col0)
	dcC, err := a.ValueDistribution("basic.yaml", "items", &col0)
	if err != nil {
		t.Fatalf("adapter ValueDistribution col: %v", err)
	}
	assertBucketsEqual(t, "items.cost", idxC, dcC)
}

func TestStatAdapter_NumericValuesParity(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())

	// amount: "oops" (anomaly) and the blank on d must both be excluded.
	idxV, _ := idxM.NumericValues("basic.yaml", "amount", nil)
	dcV, err := a.NumericValues("basic.yaml", "amount", nil)
	if err != nil {
		t.Fatalf("adapter NumericValues: %v", err)
	}
	assertFloatsEqual(t, "amount", idxV, dcV)

	col0 := 0
	idxC, _ := idxM.NumericValues("basic.yaml", "items", &col0)
	dcC, err := a.NumericValues("basic.yaml", "items", &col0)
	if err != nil {
		t.Fatalf("adapter NumericValues col: %v", err)
	}
	assertFloatsEqual(t, "items.cost", idxC, dcC)
}

func TestStatAdapter_FacetDistributionParity(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())
	idxD, _ := idxM.FacetDistribution("basic.yaml", "tier")
	dcD, err := a.FacetDistribution("basic.yaml", "tier")
	if err != nil {
		t.Fatalf("adapter FacetDistribution: %v", err)
	}
	assertBucketsEqual(t, "facet tier", idxD, dcD)
}

func TestStatAdapter_FacetCrossParity(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())
	idxC, _ := idxM.FacetCross("basic.yaml", "tier", "stage")
	dcC, err := a.FacetCross("basic.yaml", "tier", "stage")
	if err != nil {
		t.Fatalf("adapter FacetCross: %v", err)
	}
	wantC := map[[2]string]int{}
	for _, c := range idxC {
		wantC[[2]string{c.A, c.B}] = c.Count
	}
	gotC := map[[2]string]int{}
	for _, c := range dcC {
		gotC[[2]string{c.A, c.B}] = c.Count
	}
	if len(wantC) != len(gotC) {
		t.Fatalf("facet cross cells: index=%v datacore=%v", wantC, gotC)
	}
	for k, n := range wantC {
		if gotC[k] != n {
			t.Fatalf("facet cross %v: index=%d datacore=%d", k, n, gotC[k])
		}
	}
}

func TestStatAdapter_DateSeriesParity(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())
	for _, period := range []string{"year", "month", "day"} {
		idxS, _ := idxM.DateSeries("basic.yaml", "due", nil, period)
		dcS, err := a.DateSeries("basic.yaml", "due", nil, period)
		if err != nil {
			t.Fatalf("adapter DateSeries %s: %v", period, err)
		}
		assertBucketsEqual(t, "due "+period, idxS, dcS)
	}
}

func TestStatAdapter_AggregateRawParity(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())
	dims := []index.AggDim{{Kind: "field", Key: "status"}, {Kind: "facet", Key: "tier"}}
	nums := []index.AggNum{{Key: "amount"}}
	filters := []index.AggFilter{{Kind: "field", Key: "status", Op: "eq", Value: "high"}}

	idxRows, err := idxM.AggregateRaw("basic.yaml", dims, nums, filters)
	if err != nil {
		t.Fatalf("index AggregateRaw: %v", err)
	}
	dcRows, err := a.AggregateRaw("basic.yaml", dims, nums, filters)
	if err != nil {
		t.Fatalf("adapter AggregateRaw: %v", err)
	}
	want := indexRawKeys(idxRows)
	got := indexRawKeys(dcRows)
	if len(want) != len(got) {
		t.Fatalf("aggregate raw rows: index=%v datacore=%v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("aggregate raw row %d: index=%q datacore=%q", i, want[i], got[i])
		}
	}
}

// --- unhappy + boundary ---

func TestStatAdapter_EmptyTemplate(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, nil)
	if n, err := a.TotalForms("basic.yaml"); err != nil || n != 0 {
		t.Fatalf("empty TotalForms = %d err=%v, want 0", n, err)
	}
	idxD, _ := idxM.ValueDistribution("basic.yaml", "status", nil)
	dcD, _ := a.ValueDistribution("basic.yaml", "status", nil)
	assertBucketsEqual(t, "empty status", idxD, dcD)
}

func TestStatAdapter_MissingFieldIsEmpty(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())
	idxD, _ := idxM.ValueDistribution("basic.yaml", "ghost", nil)
	dcD, err := a.ValueDistribution("basic.yaml", "ghost", nil)
	if err != nil {
		t.Fatalf("adapter missing field: %v", err)
	}
	assertBucketsEqual(t, "ghost", idxD, dcD)
	if len(dcD) != 0 {
		t.Fatalf("missing field distribution = %v, want empty", dcD)
	}
}

func TestStatAdapter_UnknownColumnIsEmpty(t *testing.T) {
	a, idxM := newDatacoreStatAdapter(t, richStatFixture())
	bad := 9
	idxD, _ := idxM.ValueDistribution("basic.yaml", "items", &bad)
	dcD, err := a.ValueDistribution("basic.yaml", "items", &bad)
	if err != nil {
		t.Fatalf("adapter unknown col: %v", err)
	}
	if len(idxD) != 0 || len(dcD) != 0 {
		t.Fatalf("unknown col: index=%v datacore=%v, want both empty", idxD, dcD)
	}
}

func TestStatAdapter_TableColumnFilterErrorsNotSilent(t *testing.T) {
	a, _ := newDatacoreStatAdapter(t, richStatFixture())
	bad := 0
	_, err := a.AggregateRaw("basic.yaml",
		[]index.AggDim{{Kind: "field", Key: "status"}},
		nil,
		[]index.AggFilter{{Kind: "field", Key: "items", Col: &bad, Op: "eq", Value: "100"}},
	)
	if err == nil {
		t.Fatal("table-column filter must error (not silently under-filter)")
	}
}

func TestStatAdapter_UnknownGridColumnErrors(t *testing.T) {
	a, _ := newDatacoreStatAdapter(t, richStatFixture())
	bad := 9
	if _, err := a.AggregateRaw("basic.yaml",
		[]index.AggDim{{Kind: "field", Key: "items", Col: &bad}}, nil, nil,
	); err == nil {
		t.Fatal("unknown grid column must error loudly, not drop the axis")
	}
}

// TestStatAdapter_FacetUnsetBucketDiverges pins the one settled divergence
// (design/datacore-stat-migration.md, edge 2). A facet that is set but
// unselected is an "(unset)" row in the index, which counts it under the ""
// label; datacore treats blank as absence, so it drops it. Both engines must
// still agree on every real (non-empty) category. This is an intended
// difference, asserted as such, not a parity miss.
func TestStatAdapter_FacetUnsetBucketDiverges(t *testing.T) {
	forms := []statForm{
		{id: "a.meta.json", facets: map[string]string{"tier": "GOLD", "stage": "live"}},
		{id: "b.meta.json", facets: map[string]string{"tier": "GOLD", "stage": ""}},   // set, unselected
		{id: "c.meta.json", facets: map[string]string{"tier": "SILVER", "stage": ""}}, // set, unselected
	}
	a, idxM := newDatacoreStatAdapter(t, forms)

	idxD, _ := idxM.FacetDistribution("basic.yaml", "stage")
	dcD, err := a.FacetDistribution("basic.yaml", "stage")
	if err != nil {
		t.Fatalf("adapter FacetDistribution: %v", err)
	}
	idxMap := bucketMap(idxD)
	dcMap := bucketMap(dcD)
	if idxMap[""] != 2 {
		t.Fatalf("index stage distribution missing the (unset) bucket: %v", idxMap)
	}
	if _, has := dcMap[""]; has {
		t.Fatalf("datacore must drop the (unset) bucket, got: %v", dcMap)
	}
	if idxMap["live"] != 1 || dcMap["live"] != 1 {
		t.Fatalf("real category disagreement: index=%v datacore=%v", idxMap, dcMap)
	}

	// FacetCross carries the same divergence: a "" on either axis.
	idxC, _ := idxM.FacetCross("basic.yaml", "tier", "stage")
	dcC, err := a.FacetCross("basic.yaml", "tier", "stage")
	if err != nil {
		t.Fatalf("adapter FacetCross: %v", err)
	}
	idxEmptyCol := 0
	for _, c := range idxC {
		if c.B == "" {
			idxEmptyCol += c.Count
		}
	}
	if idxEmptyCol != 2 {
		t.Fatalf("index cross missing (unset) column cells: %v", idxC)
	}
	for _, c := range dcC {
		if c.B == "" {
			t.Fatalf("datacore cross must drop (unset) column, got cell %+v", c)
		}
	}
	if crossCount(idxC, "GOLD", "live") != 1 || crossCount(dcC, "GOLD", "live") != 1 {
		t.Fatalf("real cross cell disagreement: index=%v datacore=%v", idxC, dcC)
	}
}

func bucketMap(bs []index.Bucket) map[string]int {
	m := map[string]int{}
	for _, b := range bs {
		m[b.Label] = b.Count
	}
	return m
}

func crossCount(cells []index.CrossCell, a, b string) int {
	for _, c := range cells {
		if c.A == a && c.B == b {
			return c.Count
		}
	}
	return 0
}

// --- shared parity helpers ---

func assertBucketsEqual(t *testing.T, label string, idx, dc []index.Bucket) {
	t.Helper()
	want := map[string]int{}
	for _, b := range idx {
		want[b.Label] = b.Count
	}
	got := map[string]int{}
	for _, b := range dc {
		got[b.Label] = b.Count
	}
	if len(want) != len(got) {
		t.Fatalf("%s: bucket sets differ: index=%v datacore=%v", label, want, got)
	}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("%s: bucket %q index=%d datacore=%d", label, k, n, got[k])
		}
	}
}

func assertFloatsEqual(t *testing.T, label string, idx, dc []float64) {
	t.Helper()
	a := append([]float64{}, idx...)
	b := append([]float64{}, dc...)
	sort.Float64s(a)
	sort.Float64s(b)
	if len(a) != len(b) {
		t.Fatalf("%s: value counts differ: index=%d datacore=%d", label, len(a), len(b))
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			t.Fatalf("%s: sorted values differ at %d: index=%g datacore=%g", label, i, a[i], b[i])
		}
	}
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	f, err := parseFloat(s)
	if err != nil {
		t.Fatalf("bad float %q: %v", s, err)
	}
	return f
}
