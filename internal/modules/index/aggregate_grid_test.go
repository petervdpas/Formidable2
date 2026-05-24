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
	rows, err := m.AggregateRaw("basic.yaml", []AggDim{{Kind: "field", Key: "status"}}, []AggNum{{Key: "amount"}}, nil)
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
	rows, err := m.AggregateRaw("basic.yaml", []AggDim{{Kind: "facet", Key: "prio"}}, nil, nil)
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
	rows, err := m.AggregateRaw("basic.yaml", []AggDim{{Kind: "field", Key: "due", DateWidth: 7}}, nil, nil)
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
	rows, err := m.AggregateRaw("basic.yaml", nil, nil, nil)
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

func TestAggregateRaw_TableColumnDimension(t *testing.T) {
	m := seedValuesDB(t)
	c0 := 0 // items col0 = text "row-<file>"
	rows, err := m.AggregateRaw("basic.yaml", []AggDim{{Kind: "field", Key: "items", Col: &c0}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (one per table cell)", len(rows))
	}
	for _, r := range rows {
		if len(r.Dims) != 1 || r.Dims[0] == "" {
			t.Errorf("table-column row malformed: %+v", r)
		}
	}
}

func TestAggregateRaw_TableColumnNumericSource(t *testing.T) {
	m := seedValuesDB(t)
	c1 := 1 // items col1 = qty (number)
	rows, err := m.AggregateRaw("basic.yaml",
		[]AggDim{{Kind: "field", Key: "status"}}, []AggNum{{Key: "items", Col: &c1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := map[string]float64{}
	for _, r := range rows {
		if r.Nums[0].Valid {
			sum[r.Dims[0]] += r.Nums[0].Float64
		}
	}
	// qty: a(high)=2, b(low)=3, c(high)=5 -> high:7 low:3
	if sum["high"] != 7 || sum["low"] != 3 {
		t.Errorf("qty sum by status = %v, want high:7 low:3", sum)
	}
}

func TestAggregateRaw_FilterEquality(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml", nil, nil,
		[]AggFilter{{Kind: "field", Key: "status", Op: "eq", Value: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 { // a and c are "high"
		t.Errorf("got %d rows, want 2 (status eq high)", len(rows))
	}
}

func TestAggregateRaw_FilterNumericComparison(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml", nil, nil,
		[]AggFilter{{Kind: "field", Key: "amount", Op: "gt", Value: "15"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 { // amounts 20 and 30 are > 15
		t.Errorf("got %d rows, want 2 (amount gt 15)", len(rows))
	}
}

func TestAggregateRaw_FilterFacetEquality(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml", []AggDim{{Kind: "field", Key: "status"}}, nil,
		[]AggFilter{{Kind: "facet", Key: "prio", Op: "eq", Value: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := countByDim(rows); got["high"] != 2 || got["low"] != 0 {
		t.Errorf("filtered counts = %v, want only high:2", got)
	}
}

func TestAggregateRaw_TwoFacetCross(t *testing.T) {
	// Cross prio with itself is degenerate; instead cross the status
	// field with the prio facet (which mirror each other in the seed),
	// proving multi-dimension joins line up per form.
	m := seedValuesDB(t)
	rows, err := m.AggregateRaw("basic.yaml",
		[]AggDim{{Kind: "field", Key: "status"}, {Kind: "facet", Key: "prio"}}, nil, nil)
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
