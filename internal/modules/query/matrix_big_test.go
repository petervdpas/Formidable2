package query

import (
	"strconv"
	"testing"
)

// bigMatrix builds a deterministic matrix of forms x table-rows. Each form
// has a "region" scalar (one of R values), and an "items" table with
// rowsPerForm rows carrying a "kind" (one of K values) and a numeric
// "amount". Provenance is set so aggregates dedupe correctly. No
// randomness: every value is a pure function of the indices, so the
// independent recomputations below are exact.
func bigMatrix(forms, rowsPerForm, regions, kinds int) *Matrix {
	cols := []MatrixCol{
		{ID: sourceID(scalar("region"))},
		{ID: sourceID(tcol("items", 0))},
		{ID: sourceID(tcol("items", 1)), Hint: "number"},
	}
	m := &Matrix{Cols: cols}
	for f := range forms {
		region := "R" + strconv.Itoa(f%regions)
		form := "f" + strconv.Itoa(f) + ".json"
		for r := range rowsPerForm {
			kind := "K" + strconv.Itoa((f+r)%kinds)
			amount := (f*7 + r*13) % 100
			m.Rows = append(m.Rows, MatrixRow{
				Form:    form,
				Origins: []Origin{{Field: "items", Row: r, Count: rowsPerForm}},
				Cells:   []string{region, kind, strconv.Itoa(amount)},
			})
		}
	}
	return m
}

func TestBig_ListAndFilterMatchIndependentCount(t *testing.T) {
	m := bigMatrix(2000, 5, 8, 6) // 10000 rows
	if len(m.Rows) != 10000 {
		t.Fatalf("setup: want 10000 rows, got %d", len(m.Rows))
	}

	// Filter amount >= 50, count independently.
	want := 0
	for _, row := range m.Rows {
		if n, _ := parseNum(row.Cells[2]); n >= 50 {
			want++
		}
	}
	res, err := m.Execute(Spec{
		Columns: cols(tcol("items", 0), tcol("items", 1)),
		Filters: []Filter{{Source: tcol("items", 1), Op: "ge", Value: "50"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != want {
		t.Fatalf("filtered count = %d, want %d", res.Count, want)
	}
	if len(res.Anomalies) != 0 {
		t.Fatalf("clean data should have no anomalies, got %d", len(res.Anomalies))
	}
}

func TestBig_GroupSumMatchesIndependent(t *testing.T) {
	m := bigMatrix(1500, 4, 5, 7) // 6000 rows

	// Independent: sum of amount per region. Every (form,row) is unique
	// here (single table), so no dedupe collapse - straight sum.
	wantSum := map[string]int{}
	wantCount := map[string]int{}
	wantForms := map[string]map[string]bool{}
	for _, row := range m.Rows {
		region := row.Cells[0]
		n, _ := parseNum(row.Cells[2])
		wantSum[region] += int(n)
		wantCount[region]++
		if wantForms[region] == nil {
			wantForms[region] = map[string]bool{}
		}
		wantForms[region][row.Form] = true
	}

	res, err := m.Execute(Spec{
		Columns: cols(scalar("region"), tcol("items", 1)),
		GroupBy: []int{0},
		Measures: []Measure{
			{Func: "sum", Source: tcol("items", 1), Header: "sum"},
			{Func: "count", Header: "rows"},
			{Func: "count_distinct", Header: "forms"},
		},
		OrderBy: []Sort{{Column: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != len(wantSum) {
		t.Fatalf("group count = %d, want %d", res.Count, len(wantSum))
	}
	for _, r := range res.Rows {
		region := r[0].Text
		gotSum, _ := strconv.Atoi(r[1].Text)
		gotRows, _ := strconv.Atoi(r[2].Text)
		gotForms, _ := strconv.Atoi(r[3].Text)
		if gotSum != wantSum[region] {
			t.Fatalf("region %s sum = %d, want %d", region, gotSum, wantSum[region])
		}
		if gotRows != wantCount[region] {
			t.Fatalf("region %s rows = %d, want %d", region, gotRows, wantCount[region])
		}
		if gotForms != len(wantForms[region]) {
			t.Fatalf("region %s forms = %d, want %d", region, gotForms, len(wantForms[region]))
		}
	}
}

func TestBig_DistinctMatchesIndependent(t *testing.T) {
	m := bigMatrix(1000, 6, 4, 9) // 6000 rows
	seen := map[string]bool{}
	for _, row := range m.Rows {
		seen[row.Cells[0]+"\x1f"+row.Cells[1]] = true
	}
	res, _ := m.Execute(Spec{
		Columns:  cols(scalar("region"), tcol("items", 0)),
		Distinct: true,
	})
	if res.Count != len(seen) {
		t.Fatalf("distinct (region,kind) = %d, want %d", res.Count, len(seen))
	}
}

func TestBig_NumericSortIsOrdered(t *testing.T) {
	m := bigMatrix(800, 5, 3, 5) // 4000 rows
	res, _ := m.Execute(Spec{
		Columns: cols(tcol("items", 1)),
		OrderBy: []Sort{{Column: 0, Numeric: true}},
	})
	prev := -1.0
	for _, r := range res.Rows {
		n, ok := parseNum(r[0].Text)
		if !ok {
			t.Fatalf("non-numeric cell in numeric column: %q", r[0].Text)
		}
		if n < prev {
			t.Fatalf("numeric sort out of order: %v after %v", n, prev)
		}
		prev = n
	}
}

func TestBig_LimitTruncates(t *testing.T) {
	m := bigMatrix(500, 4, 3, 5)
	res, _ := m.Execute(Spec{Columns: cols(tcol("items", 0)), Limit: 100})
	if res.Count != 100 {
		t.Fatalf("limit 100: got %d", res.Count)
	}
}
