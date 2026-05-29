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

func TestGridRank0IsOneRowPerForm(t *testing.T) {
	// No dims, no nums: a row per form (filename leads), like the index.
	rows := gridFixture().View().Grid(nil, nil, nil)
	if len(rows) != 4 {
		t.Fatalf("rank-0 rows = %d, want 4", len(rows))
	}
	_ = gridKeys(rows) // exercise the key helper
}
