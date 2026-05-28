package index

import "testing"

// cellText pulls the text values of one projected row into a slice, for
// compact assertions.
func cellText(r ProjectRow) []string {
	out := make([]string, len(r.Cells))
	for i, c := range r.Cells {
		out[i] = c.Text
	}
	return out
}

func TestProjectRows_ScalarFields(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols: []ProjectCol{{Kind: "field", Key: "status"}, {Kind: "field", Key: "amount"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (one per form)", len(rows))
	}
	byForm := map[string][]string{}
	for _, r := range rows {
		if r.Form == "" {
			t.Errorf("row missing filename: %+v", r)
		}
		byForm[r.Form] = cellText(r)
	}
	if got := byForm["a.meta.json"]; got[0] != "high" || got[1] != "10" {
		t.Errorf("a.meta.json = %v, want [high 10]", got)
	}
}

func TestProjectRows_Filter(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "status"}},
		Filters: []AggFilter{{Kind: "field", Key: "status", Op: "eq", Value: "high"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 high", len(rows))
	}
	for _, r := range rows {
		if r.Cells[0].Text != "high" {
			t.Errorf("unexpected row %v", cellText(r))
		}
	}
}

func TestProjectRows_NumericFilter(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "amount"}},
		Filters: []AggFilter{{Kind: "field", Key: "amount", Op: "ge", Value: "20"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (amount>=20)", len(rows))
	}
}

func TestProjectRows_FacetColumn(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols: []ProjectCol{{Kind: "facet", Key: "prio"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, r := range rows {
		got[r.Cells[0].Text]++
	}
	if got["high"] != 2 || got["low"] != 1 {
		t.Errorf("facet projection = %v, want high:2 low:1", got)
	}
}

// A table column fans to one row per cell, carrying the source filename -
// this is the flatten-to-rows view of a table field.
func TestProjectRows_TableColumnFan(t *testing.T) {
	m := seedValuesDB(t)
	c0 := 0
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols: []ProjectCol{{Kind: "field", Key: "items", Col: &c0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (one per items cell)", len(rows))
	}
}

// Distinct drops the per-form identity and collapses the projected tuple
// to its unique values - the "flatten list/table and select distinct"
// case.
func TestProjectRows_Distinct(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols:     []ProjectCol{{Kind: "field", Key: "status"}},
		Distinct: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d distinct rows, want 2 (high, low)", len(rows))
	}
	for _, r := range rows {
		if r.Form != "" {
			t.Errorf("distinct row should carry no filename, got %q", r.Form)
		}
	}
}

func TestProjectRows_OrderByNumericDescLimit(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "amount"}},
		OrderBy: []ProjectSort{{Index: 0, Desc: true, Numeric: true}},
		Limit:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (limit)", len(rows))
	}
	if rows[0].Cells[0].Text != "30" || rows[1].Cells[0].Text != "20" {
		t.Errorf("order = %v %v, want 30 then 20", rows[0].Cells[0].Text, rows[1].Cells[0].Text)
	}
}

// A projected column the form has no value for still yields a row (LEFT
// JOIN), with an empty, num-invalid cell - projection must not silently
// drop forms the way a grouping INNER JOIN does.
func TestProjectRows_MissingValueLeftJoin(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols: []ProjectCol{{Kind: "field", Key: "amount"}, {Kind: "field", Key: "nope"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (missing column must not drop forms)", len(rows))
	}
	for _, r := range rows {
		if r.Cells[1].Text != "" || r.Cells[1].Num.Valid {
			t.Errorf("missing cell should be empty+invalid, got %+v", r.Cells[1])
		}
	}
}

func TestProjectRows_BadNumericFilter(t *testing.T) {
	m := seedValuesDB(t)
	_, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "amount"}},
		Filters: []AggFilter{{Kind: "field", Key: "amount", Op: "gt", Value: "notanumber"}},
	})
	if err == nil {
		t.Fatal("expected error for non-numeric filter value")
	}
}

func TestProjectRows_UnknownFilterOp(t *testing.T) {
	m := seedValuesDB(t)
	_, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "status"}},
		Filters: []AggFilter{{Kind: "field", Key: "status", Op: "weird", Value: "x"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown filter op")
	}
}
