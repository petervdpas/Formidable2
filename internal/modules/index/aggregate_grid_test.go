package index

import "testing"

// countByDim collapses raw rows into a label->count map over the first
// dimension, for assertions that don't care about numeric values.
func countByDim(rows []StatRawRow) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		if len(r.Dims) > 0 {
			out[r.Dims[0]]++
		}
	}
	return out
}

func TestAggregateRaw_ScalarFieldWithNumericSource(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml", []AggDim{{Kind: "field", Key: "status"}}, []string{"amount"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (one per form)", len(rows))
	}
	// Each row carries its status label and its amount.
	sumByStatus := map[string]float64{}
	for _, r := range rows {
		if len(r.Dims) != 1 || len(r.Nums) != 1 {
			t.Fatalf("row shape wrong: %+v", r)
		}
		if !r.Nums[0].Valid {
			t.Errorf("amount missing for %v", r.Dims)
		}
		sumByStatus[r.Dims[0]] += r.Nums[0].Float64
	}
	if sumByStatus["high"] != 40 || sumByStatus["low"] != 20 {
		t.Errorf("sum by status = %v, want high:40 low:20", sumByStatus)
	}
}

func TestAggregateRaw_FacetDimension(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml", []AggDim{{Kind: "facet", Key: "prio"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := countByDim(rows)
	if got["high"] != 2 || got["low"] != 1 {
		t.Errorf("facet counts = %v, want high:2 low:1", got)
	}
}

func TestAggregateRaw_DateBinMonth(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml", []AggDim{{Kind: "field", Key: "due", DateWidth: 7}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := countByDim(rows)
	if got["2026-01"] != 2 || got["2026-02"] != 1 {
		t.Errorf("month buckets = %v, want 2026-01:2 2026-02:1", got)
	}
}

func TestAggregateRaw_Rank0OneRowPerForm(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (one per form)", len(rows))
	}
	for _, r := range rows {
		if len(r.Dims) != 0 || len(r.Nums) != 0 {
			t.Errorf("rank-0 row should be empty, got %+v", r)
		}
	}
}

func TestAggregateRaw_TwoFacetCross(t *testing.T) {
	// Cross prio with itself is degenerate; instead cross the status
	// field with the prio facet (which mirror each other in the seed),
	// proving multi-dimension joins line up per form.
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml",
		[]AggDim{{Kind: "field", Key: "status"}, {Kind: "facet", Key: "prio"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	for _, r := range rows {
		if len(r.Dims) != 2 || r.Dims[0] != r.Dims[1] {
			t.Errorf("status and prio should match per form, got %v", r.Dims)
		}
	}
}
