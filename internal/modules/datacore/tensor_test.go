package datacore

import (
	"strconv"
	"testing"
)

func sampleRecords() []Record {
	return []Record{
		{ID: "a", Fields: map[string]string{"team": "east", "amount": "10"}, Facets: map[string]string{"status": "active"}},
		{ID: "b", Fields: map[string]string{"team": "east", "amount": "30"}, Facets: map[string]string{"status": "retired"}},
		{ID: "c", Fields: map[string]string{"team": "west", "amount": "20"}, Facets: map[string]string{"status": "active"}},
	}
}

func ingestAll(t *Tensor, recs []Record) {
	for _, r := range recs {
		t.Ingest(r)
	}
}

func TestIngestPlacesScalarsAndFacets(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	// 3 records x (team + amount + facet:status) = 9 cells, no tables.
	if dt.Len() != 9 {
		t.Fatalf("Len = %d, want 9", dt.Len())
	}
	v, _, ok := dt.at(dt.iax.intern("a"), dt.fax.intern("facet:status"), dt.max.intern(Universal))
	if !ok || v != "active" {
		t.Fatalf("facet a = (%q,%v), want active,true", v, ok)
	}
}

func TestProjectIsStableBySortedIdentity(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	rows := dt.View().Project("team", "amount")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	want := []Row{
		{ID: "a", Cells: []string{"east", "10"}},
		{ID: "b", Cells: []string{"east", "30"}},
		{ID: "c", Cells: []string{"west", "20"}},
	}
	for i, w := range want {
		if rows[i].ID != w.ID || rows[i].Cells[0] != w.Cells[0] || rows[i].Cells[1] != w.Cells[1] {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], w)
		}
	}
}

func TestProjectUnknownFieldReadsBlank(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	rows := dt.View().Project("team", "nope")
	for _, r := range rows {
		if r.Cells[1] != "" {
			t.Fatalf("unknown field for %s = %q, want blank", r.ID, r.Cells[1])
		}
	}
}

func TestWhereNarrowsIdentities(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	rows := dt.View().Where("team", func(v string) bool { return v == "east" }).Project("amount")
	if len(rows) != 2 {
		t.Fatalf("east rows = %d, want 2", len(rows))
	}
	if rows[0].ID != "a" || rows[1].ID != "b" {
		t.Fatalf("east ids = %s,%s want a,b", rows[0].ID, rows[1].ID)
	}
}

func TestWhereOnUnknownFieldYieldsEmpty(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	if n := dt.View().Where("ghost", func(string) bool { return true }).Count(); n != 0 {
		t.Fatalf("count over unknown field = %d, want 0", n)
	}
}

func TestDistributionReducesAlongIdentity(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	got := dt.View().Distribution("team")
	want := []Bucket{{Value: "east", Count: 2}, {Value: "west", Count: 1}}
	if len(got) != len(want) {
		t.Fatalf("buckets = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("bucket %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestDistributionSumEqualsCount(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	v := dt.View()
	sum := 0
	for _, b := range v.Distribution("team") {
		sum += b.Count
	}
	if sum != v.Count() {
		t.Fatalf("distribution sum %d != count %d", sum, v.Count())
	}
}

func TestScopeMissingContextYieldsBlank(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	rows := dt.View().Scope("temporal").Project("team")
	for _, r := range rows {
		if r.Cells[0] != "" {
			t.Fatalf("value at empty scope = %q, want blank", r.Cells[0])
		}
	}
}

// A moderate deterministic corpus: the distribution must partition the
// identities exactly, with no double counting from the columnar scan.
func TestDistributionPartitionsLargeCorpus(t *testing.T) {
	dt := New()
	const n = 600
	for i := range n {
		team := "east"
		if i%3 == 0 {
			team = "west"
		}
		dt.Ingest(Record{ID: "r" + strconv.Itoa(i), Fields: map[string]string{"team": team}})
	}
	v := dt.View()
	if v.Count() != n {
		t.Fatalf("count = %d, want %d", v.Count(), n)
	}
	dist := dt.View().Distribution("team")
	byVal := map[string]int{}
	for _, b := range dist {
		byVal[b.Value] = b.Count
	}
	wantWest := (n + 2) / 3 // i%3==0 over [0,n)
	if byVal["west"] != wantWest || byVal["east"] != n-wantWest {
		t.Fatalf("dist = %v, want west=%d east=%d", byVal, wantWest, n-wantWest)
	}
}
