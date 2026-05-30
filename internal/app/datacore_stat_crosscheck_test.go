package app

import (
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

// TestDatacore_AggregateValuesMatchIndexNumericValues proves Aggregate.Values
// carries the same raw numbers stat reads from index.NumericValues, so the
// stat layer can compute median/stddev/percentile off the tensor without a
// second extraction. Order differs (datacore is working-set order, the index
// has no ORDER BY), so the two are compared as multisets; the median computed
// from each must then agree, which is the property stat actually needs.
func TestDatacore_AggregateValuesMatchIndexNumericValues(t *testing.T) {
	type form struct {
		id     string
		amount float64
		costs  []float64
	}
	fixture := []form{
		{"a.meta.json", 10, []float64{100, 50}},
		{"b.meta.json", 30, []float64{25}},
		{"c.meta.json", 20, []float64{200, 5, 5}},
		{"d.meta.json", 40, nil},
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

	assertSameValues(t, "amount (scalar)", idxM, "amount", nil, dt.View().Aggregate("amount").Values)
	assertSameValues(t, "cost (table column)", idxM, "items", &col0, dt.View().Follow("items").Aggregate("cost").Values)
}

func assertSameValues(t *testing.T, label string, idxM *index.Manager, field string, col *int, dc []float64) {
	t.Helper()
	idx, err := idxM.NumericValues("basic.yaml", field, col)
	if err != nil {
		t.Fatalf("%s: index NumericValues: %v", label, err)
	}
	gotIdx := append([]float64{}, idx...)
	gotDC := append([]float64{}, dc...)
	sort.Float64s(gotIdx)
	sort.Float64s(gotDC)
	if len(gotIdx) != len(gotDC) {
		t.Fatalf("%s: value count index=%d datacore=%d", label, len(gotIdx), len(gotDC))
	}
	for i := range gotIdx {
		if math.Abs(gotIdx[i]-gotDC[i]) > 1e-9 {
			t.Fatalf("%s: sorted values differ at %d: index=%g datacore=%g", label, i, gotIdx[i], gotDC[i])
		}
	}
	if math.Abs(median(gotIdx)-median(gotDC)) > 1e-9 {
		t.Fatalf("%s: median index=%g datacore=%g", label, median(gotIdx), median(gotDC))
	}
}

// median of an already-sorted slice; the order-independent statistic stat
// needs Values for. Empty slice is 0.
func median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
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

// TestDatacore_DateSeriesMatchesIndex proves the date histogram agrees with
// index.DateSeries for every period. index buckets the stored ISO date by a
// prefix width; datacore parses the date and formats it to the period. On
// real ISO dates the two must produce identical buckets.
func TestDatacore_DateSeriesMatchesIndex(t *testing.T) {
	dates := map[string]string{
		"a.meta.json": "2026-01-15",
		"b.meta.json": "2026-01-20",
		"c.meta.json": "2026-02-05",
		"d.meta.json": "2027-03-01",
	}

	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	var forms []index.FormRow
	for file, due := range dates {
		forms = append(forms, index.FormRow{
			Template: "basic.yaml", Filename: file, Mtime: 100,
			Values: []index.FormValueRow{{FieldKey: "due", ValueType: "date", Text: due}},
		})
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	dt := datacore.New()
	for file, due := range dates {
		dt.Ingest(datacore.Record{ID: file, Fields: map[string]string{"due": due}})
	}

	for _, period := range []string{"year", "month", "day"} {
		idxSeries, err := idxM.DateSeries("basic.yaml", "due", nil, period)
		if err != nil {
			t.Fatalf("index date series %s: %v", period, err)
		}
		dcSeries := dt.View().DateSeries("due", period)

		want := map[string]int{}
		for _, b := range idxSeries {
			want[b.Label] = b.Count
		}
		got := map[string]int{}
		for _, b := range dcSeries.Buckets {
			got[b.Value] = b.Count
		}
		if len(want) != len(got) {
			t.Fatalf("%s: bucket counts differ: index=%v datacore=%v", period, want, got)
		}
		for k, n := range want {
			if got[k] != n {
				t.Fatalf("%s: bucket %q index=%d datacore=%d", period, k, n, got[k])
			}
		}
	}
}

// TestDatacore_AggregateRawMatchesIndex proves the raw grid agrees with
// index.AggregateRaw for the scalar/facet/date core: two dims (region field +
// tier facet), one numeric measure (amount), filtered to active status. Both
// engines must emit the same (form, dims, nums) rows. Table-column dims are
// excluded here on purpose (datacore aligns rows where the index fans).
func TestDatacore_AggregateRawMatchesIndex(t *testing.T) {
	type form struct {
		id, region, tier, status string
		amount                   float64
		hasAmount                bool
	}
	fixture := []form{
		{"a.meta.json", "east", "GOLD", "active", 10, true},
		{"b.meta.json", "east", "SILVER", "active", 30, true},
		{"c.meta.json", "west", "GOLD", "retired", 20, true},
		{"d.meta.json", "west", "GOLD", "active", 0, false}, // no amount
	}

	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	var forms []index.FormRow
	for _, f := range fixture {
		vals := []index.FormValueRow{
			{FieldKey: "region", ValueType: "text", Text: f.region},
			{FieldKey: "status", ValueType: "text", Text: f.status},
		}
		if f.hasAmount {
			amt := f.amount
			vals = append(vals, index.FormValueRow{FieldKey: "amount", ValueType: "number", Num: &amt, Text: ftoa(amt)})
		}
		forms = append(forms, index.FormRow{
			Template: "basic.yaml", Filename: f.id, Mtime: 100,
			Facets: []index.FormFacet{{Key: "tier", Set: true, Selected: f.tier}},
			Values: vals,
		})
	}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	idxRows, err := idxM.AggregateRaw("basic.yaml",
		[]index.AggDim{{Kind: "field", Key: "region"}, {Kind: "facet", Key: "tier"}},
		[]index.AggNum{{Key: "amount"}},
		[]index.AggFilter{{Kind: "field", Key: "status", Op: "eq", Value: "active"}},
	)
	if err != nil {
		t.Fatalf("index AggregateRaw: %v", err)
	}

	dt := datacore.New()
	for _, f := range fixture {
		fields := map[string]string{"region": f.region, "status": f.status}
		if f.hasAmount {
			fields["amount"] = ftoa(f.amount)
		}
		dt.Ingest(datacore.Record{ID: f.id, Fields: fields, Facets: map[string]string{"tier": f.tier}})
	}
	dcRows := dt.View().Grid(
		[]datacore.GridDim{{Field: "region"}, {Field: "facet:tier"}},
		[]datacore.GridNum{{Field: "amount"}},
		[]datacore.GridFilter{{Field: "status", Op: "eq", Value: "active"}},
	)

	want := indexRawKeys(idxRows)
	got := datacoreRawKeys(dcRows)
	if len(want) != len(got) {
		t.Fatalf("row count differs: index=%v datacore=%v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("row %d differs: index=%q datacore=%q", i, want[i], got[i])
		}
	}
}

// TestDatacore_TableColumnFanAlignsWhereIndexCartesians is the divergence gap 2
// closes, made concrete. Two columns of one table (category, vendor) over a
// two-row table describe two real pairs: (food, acme) and (travel, rail). The
// index stores each column as (field_key, col) with no row index, so crossing
// the two columns self-joins into all 4 combinations (a cartesian over-count).
// datacore keeps each table row as its own identity and reads both columns off
// it, so it emits exactly the 2 real pairs. This is a deliberate divergence,
// not a parity miss: datacore is correct where the index is not.
func TestDatacore_TableColumnFanAlignsWhereIndexCartesians(t *testing.T) {
	idxM, err := index.NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	col0, col1 := 0, 1
	forms := []index.FormRow{{
		Template: "basic.yaml", Filename: "a.meta.json", Mtime: 100,
		Values: []index.FormValueRow{
			{FieldKey: "items", Col: &col0, ValueType: "text", Text: "food"},   // row 0 category
			{FieldKey: "items", Col: &col0, ValueType: "text", Text: "travel"}, // row 1 category
			{FieldKey: "items", Col: &col1, ValueType: "text", Text: "acme"},   // row 0 vendor
			{FieldKey: "items", Col: &col1, ValueType: "text", Text: "rail"},   // row 1 vendor
		},
	}}
	if err := index.Reconcile(idxM.DB(), index.ReconcileBatch{
		UpsertTemplates: []index.TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	idxRows, err := idxM.AggregateRaw("basic.yaml",
		[]index.AggDim{{Kind: "field", Key: "items", Col: &col0}, {Kind: "field", Key: "items", Col: &col1}},
		nil, nil,
	)
	if err != nil {
		t.Fatalf("index AggregateRaw: %v", err)
	}
	if len(idxRows) != 4 {
		t.Fatalf("index rows = %d, want 4 (the cartesian over-count datacore avoids)", len(idxRows))
	}

	dt := datacore.New()
	dt.Ingest(datacore.Record{ID: "a.meta.json", Tables: map[string][]map[string]string{
		"items": {
			{"category": "food", "vendor": "acme"},
			{"category": "travel", "vendor": "rail"},
		},
	}})
	dcRows := dt.View().Grid(
		[]datacore.GridDim{{Field: "category", Table: "items"}, {Field: "vendor", Table: "items"}},
		nil, nil,
	)
	if len(dcRows) != 2 {
		t.Fatalf("datacore rows = %d, want 2 (aligned, no cartesian)", len(dcRows))
	}
	pairs := map[string]bool{}
	for _, r := range dcRows {
		pairs[r.Dims[0]+"/"+r.Dims[1]] = true
	}
	if !pairs["food/acme"] || !pairs["travel/rail"] || len(pairs) != 2 {
		t.Fatalf("datacore pairs = %v, want exactly food/acme + travel/rail", pairs)
	}
}

func indexRawKeys(rows []index.StatRawRow) []string {
	ks := make([]string, len(rows))
	for i, r := range rows {
		num := "∅"
		if len(r.Nums) > 0 && r.Nums[0].Valid {
			num = strconv.FormatFloat(r.Nums[0].Float64, 'f', -1, 64)
		}
		ks[i] = r.Form + "|" + strings.Join(r.Dims, ",") + "|" + num
	}
	sort.Strings(ks)
	return ks
}

func datacoreRawKeys(rows []datacore.GridRow) []string {
	ks := make([]string, len(rows))
	for i, r := range rows {
		num := "∅"
		if len(r.Nums) > 0 && r.Nums[0].OK {
			num = strconv.FormatFloat(r.Nums[0].Value, 'f', -1, 64)
		}
		ks[i] = r.Form + "|" + strings.Join(r.Dims, ",") + "|" + num
	}
	sort.Strings(ks)
	return ks
}

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
