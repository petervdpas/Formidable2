package datacore

import (
	"sort"
	"strconv"
	"strings"
	"testing"
)

func gridFixture() *Tensor {
	dt := New()
	recs := []Record{
		{ID: "a", Fields: map[string]string{"region": "east", "amount": "10", "status": "active"}, Facets: map[string]string{"tier": "gold"}},
		{ID: "b", Fields: map[string]string{"region": "east", "amount": "30", "status": "active"}, Facets: map[string]string{"tier": "silver"}},
		{ID: "c", Fields: map[string]string{"region": "west", "amount": "20", "status": "retired"}, Facets: map[string]string{"tier": "gold"}},
		{ID: "d", Fields: map[string]string{"region": "west", "status": "active"}, Facets: map[string]string{"tier": "gold"}}, // no amount
	}
	for _, r := range recs {
		dt.Ingest(r)
	}
	return dt
}

// key renders a row to a comparable string for set comparison.
func gridKey(r GridRow) string {
	var b strings.Builder
	b.WriteString(r.Form)
	b.WriteString("|")
	b.WriteString(strings.Join(r.Dims, ","))
	b.WriteString("|")
	for _, n := range r.Nums {
		if n.OK {
			b.WriteString(strconv.FormatFloat(n.Value, 'f', -1, 64))
		} else {
			b.WriteString("∅")
		}
		b.WriteString(";")
	}
	return b.String()
}

func gridKeys(rows []GridRow) []string {
	ks := make([]string, len(rows))
	for i, r := range rows {
		ks[i] = gridKey(r)
	}
	sort.Strings(ks)
	return ks
}

func TestGridScalarAndFacetDims(t *testing.T) {
	rows := gridFixture().View().Grid(
		[]GridDim{{Field: "region"}, {Field: "facet:tier"}},
		nil, nil,
	)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4 (one per form)", len(rows))
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.Form+":"+strings.Join(r.Dims, "/")] = true
	}
	if !got["a:east/gold"] || !got["b:east/silver"] || !got["c:west/gold"] || !got["d:west/gold"] {
		t.Fatalf("dim rows = %v", got)
	}
}

func TestGridNumLeftJoinNull(t *testing.T) {
	// d has no amount: its num cell must be NOT OK, but the row still appears.
	rows := gridFixture().View().Grid([]GridDim{{Field: "region"}}, []GridNum{{Field: "amount"}}, nil)
	var d GridRow
	for _, r := range rows {
		if r.Form == "d" {
			d = r
		}
	}
	if d.Form != "d" {
		t.Fatal("form d missing; LEFT-join num must not drop the row")
	}
	if d.Nums[0].OK {
		t.Fatalf("d amount = %+v, want not OK (null)", d.Nums[0])
	}
}

func TestGridDimCompleteCaseDrops(t *testing.T) {
	// Group by amount (a dim): d has no amount, so it drops entirely.
	rows := gridFixture().View().Grid([]GridDim{{Field: "amount"}}, nil, nil)
	for _, r := range rows {
		if r.Form == "d" {
			t.Fatal("d has no amount; as a dim it must drop the row (INNER)")
		}
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (d dropped)", len(rows))
	}
}

func TestGridFilterScopesRoots(t *testing.T) {
	rows := gridFixture().View().Grid(
		[]GridDim{{Field: "region"}},
		nil,
		[]GridFilter{{Field: "status", Op: "eq", Value: "active"}},
	)
	forms := map[string]bool{}
	for _, r := range rows {
		forms[r.Form] = true
	}
	if len(forms) != 3 || forms["c"] {
		t.Fatalf("active forms = %v, want a,b,d (c retired)", forms)
	}
}

func TestGridNumericFilter(t *testing.T) {
	rows := gridFixture().View().Grid(
		[]GridDim{{Field: "region"}},
		nil,
		[]GridFilter{{Field: "amount", Op: "gt", Value: "15"}},
	)
	forms := map[string]bool{}
	for _, r := range rows {
		forms[r.Form] = true
	}
	// amount>15: b(30), c(20). a(10) out, d(no amount) out.
	if len(forms) != 2 || !forms["b"] || !forms["c"] {
		t.Fatalf("amount>15 forms = %v, want b,c", forms)
	}
}

func TestGridDateDim(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "a", Fields: map[string]string{"due": "2026-01-15"}})
	dt.Ingest(Record{ID: "b", Fields: map[string]string{"due": "2026-01-28"}})
	dt.Ingest(Record{ID: "c", Fields: map[string]string{"due": "2027-02-01"}})

	rows := dt.View().Grid([]GridDim{{Field: "due", DateWidth: 7}}, nil, nil)
	buckets := map[string]int{}
	for _, r := range rows {
		buckets[r.Dims[0]]++
	}
	if buckets["2026-01"] != 2 || buckets["2027-02"] != 1 {
		t.Fatalf("date-dim buckets = %v, want 2026-01:2 2027-02:1", buckets)
	}
}

func TestGridDateDimDropsNonDate(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "a", Fields: map[string]string{"due": "2026-01-15"}})
	dt.Ingest(Record{ID: "b", Fields: map[string]string{"due": "whenever"}})

	rows := dt.View().Grid([]GridDim{{Field: "due", DateWidth: 7}}, nil, nil)
	if len(rows) != 1 || rows[0].Form != "a" {
		t.Fatalf("rows = %v, want only a (b's due is not a date, dropped)", rows)
	}
}

// tableFixture: two forms, each with an "items" table whose rows carry a
// category and a cost, plus a root region. The grid should fan per table row.
func tableFixture() *Tensor {
	dt := New()
	dt.Ingest(Record{
		ID:     "f1",
		Fields: map[string]string{"region": "east"},
		Tables: map[string][]map[string]string{"items": {
			{"category": "food", "cost": "10"},
			{"category": "travel", "cost": "20"},
		}},
	})
	dt.Ingest(Record{
		ID:     "f2",
		Fields: map[string]string{"region": "west"},
		Tables: map[string][]map[string]string{"items": {
			{"category": "food", "cost": "5"},
		}},
	})
	return dt
}

// A table-column dim fans one row per table row, and a table-column measure is
// read off the SAME row, so category and cost stay aligned per row.
func TestGridFansTableColumnRowAligned(t *testing.T) {
	rows := tableFixture().View().Grid(
		[]GridDim{{Field: "category", Table: "items"}},
		[]GridNum{{Field: "cost", Table: "items"}},
		nil,
	)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (2 from f1, 1 from f2)", len(rows))
	}
	got := gridKeys(rows)
	want := []string{"f1|food|10;", "f1|travel|20;", "f2|food|5;"}
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

// Two columns of the SAME table must stay paired per row, not cross-produce.
// This is the property the index gets wrong (same-table self-join cartesian);
// datacore reads both off one row identity, so only the real pairs appear.
func TestGridTwoTableColumnsStayAlignedNotCartesian(t *testing.T) {
	dt := New()
	dt.Ingest(Record{
		ID: "f1",
		Tables: map[string][]map[string]string{"items": {
			{"category": "food", "vendor": "acme"},
			{"category": "travel", "vendor": "rail"},
		}},
	})

	rows := dt.View().Grid(
		[]GridDim{{Field: "category", Table: "items"}, {Field: "vendor", Table: "items"}},
		nil, nil,
	)
	got := gridKeys(rows)
	// Real pairs only: food/acme and travel/rail. NOT food/rail or travel/acme.
	want := []string{"f1|food,acme|", "f1|travel,rail|"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("rows = %v, want %v (no cartesian)", got, want)
	}
}

// A root-scoped dim is broadcast onto every fanned table row.
func TestGridMixesRootAndTableColumns(t *testing.T) {
	rows := tableFixture().View().Grid(
		[]GridDim{{Field: "region"}, {Field: "category", Table: "items"}},
		[]GridNum{{Field: "cost", Table: "items"}},
		nil,
	)
	got := gridKeys(rows)
	want := []string{"f1|east,food|10;", "f1|east,travel|20;", "f2|west,food|5;"}
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

// A root whose fan table has no rows contributes nothing (complete-case fan).
func TestGridRootWithNoTableRowsDrops(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "f1", Tables: map[string][]map[string]string{"items": {{"category": "food"}}}})
	dt.Ingest(Record{ID: "f2", Fields: map[string]string{"region": "west"}}) // no items table

	rows := dt.View().Grid([]GridDim{{Field: "category", Table: "items"}}, nil, nil)
	if len(rows) != 1 || rows[0].Form != "f1" {
		t.Fatalf("rows = %v, want only f1 (f2 has no items rows)", gridKeys(rows))
	}
}

// A table-row missing the dim column drops just that row, not the whole form.
func TestGridTableRowMissingDimDrops(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "f1", Tables: map[string][]map[string]string{"items": {
		{"category": "food", "cost": "10"},
		{"cost": "20"}, // no category
	}}})

	rows := dt.View().Grid([]GridDim{{Field: "category", Table: "items"}}, []GridNum{{Field: "cost", Table: "items"}}, nil)
	if len(rows) != 1 || rows[0].Dims[0] != "food" {
		t.Fatalf("rows = %v, want only the food row (the category-less row drops)", gridKeys(rows))
	}
}

// Naming two different tables is invalid: Grid yields nothing, and the Service
// surfaces a descriptive error rather than silently returning empty.
func TestGridMultiTableIsInvalid(t *testing.T) {
	dt := tableFixture()
	rows := dt.View().Grid(
		[]GridDim{{Field: "category", Table: "items"}, {Field: "x", Table: "other"}},
		nil, nil,
	)
	if rows != nil {
		t.Fatalf("multi-table grid = %v, want nil", rows)
	}

	svc := NewService(func(string) Loader { return staticLoader{dt: dt} })
	_, err := svc.AggregateRaw("t",
		[]GridDim{{Field: "category", Table: "items"}, {Field: "x", Table: "other"}},
		nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "one table") {
		t.Fatalf("multi-table AggregateRaw err = %v, want a single-table error", err)
	}
}

// staticLoader yields a prebuilt tensor's records by re-emitting nothing; the
// Service builds from Records, so we expose the fixture's records via a small
// loader. Here the loader returns no records (the error fires before Build), so
// an empty set is enough to reach the validation branch.
type staticLoader struct{ dt *Tensor }

func (staticLoader) Records() ([]Record, error) { return nil, nil }

func TestGridRank0IsOneRowPerForm(t *testing.T) {
	// No dims, no nums: a row per form (filename leads), like the index.
	rows := gridFixture().View().Grid(nil, nil, nil)
	if len(rows) != 4 {
		t.Fatalf("rank-0 rows = %d, want 4", len(rows))
	}
	_ = gridKeys(rows) // exercise the key helper
}
