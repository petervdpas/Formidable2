package query

import (
	"testing"
)

// ptr is a small helper for the optional Col index.
func ptr(i int) *int { return &i }

func scalar(key string) Source      { return Source{Kind: "field", Key: key} }
func tcol(key string, c int) Source { return Source{Kind: "field", Key: key, Col: ptr(c)} }
func facet(key string) Source       { return Source{Kind: "facet", Key: key} }

// build a small matrix: an "apps" table (col0 name, col1 cost) plus a
// scalar "team" and a facet "tier". Two forms, each with two table rows,
// so same-table columns must stay aligned row-for-row.
func sampleMatrix() *Matrix {
	cols := []MatrixCol{
		{ID: sourceID(scalar("team"))},
		{ID: sourceID(tcol("apps", 0))},
		{ID: sourceID(tcol("apps", 1)), Hint: "number"},
		{ID: sourceID(facet("tier"))},
	}
	apps := func(form, team, name, cost, tier string, row int) MatrixRow {
		return MatrixRow{
			Form:    form,
			Origins: []Origin{{Field: "apps", Row: row, Count: 2}},
			Cells:   []string{team, name, cost, tier},
		}
	}
	rows := []MatrixRow{
		apps("a.json", "Alpha", "billing", "100", "gold", 0),
		apps("a.json", "Alpha", "search", "40", "gold", 1),
		apps("b.json", "Beta", "billing", "10", "silver", 0),
		apps("b.json", "Beta", "report", "200", "silver", 1),
	}
	return &Matrix{Cols: cols, Rows: rows}
}

func cols(srcs ...Source) []Column {
	out := make([]Column, len(srcs))
	for i, s := range srcs {
		out[i] = Column{Header: s.Key, Source: s}
	}
	return out
}

func TestExecute_SameTableColumnsStayAligned(t *testing.T) {
	m := sampleMatrix()
	res, err := m.Execute(Spec{Columns: cols(tcol("apps", 0), tcol("apps", 1))})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 4 {
		t.Fatalf("want 4 rows, got %d", res.Count)
	}
	// The bug this engine fixes: billing pairs with 100 and 10, search with
	// 40, report with 200. No cartesian, no misalignment.
	want := map[string]string{"billing": "100", "search": "40", "report": "200"}
	for _, r := range res.Rows {
		name, cost := r[0].Text, r[1].Text
		if name == "billing" {
			if cost != "100" && cost != "10" {
				t.Fatalf("billing cost misaligned: %s", cost)
			}
			continue
		}
		if want[name] != cost {
			t.Fatalf("%s expected %s, got %s", name, want[name], cost)
		}
	}
}

func TestExecute_NumericFilterCoerces(t *testing.T) {
	m := sampleMatrix()
	res, err := m.Execute(Spec{
		Columns: cols(tcol("apps", 0), tcol("apps", 1)),
		Filters: []Filter{{Source: tcol("apps", 1), Op: "ge", Value: "100"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 2 {
		t.Fatalf("want 2 rows (>=100), got %d", res.Count)
	}
}

func TestExecute_StringFilterEq(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{
		Columns: cols(tcol("apps", 0)),
		Filters: []Filter{{Source: scalar("team"), Op: "eq", Value: "Beta"}},
	})
	if res.Count != 2 {
		t.Fatalf("want 2 Beta rows, got %d", res.Count)
	}
}

func TestExecute_Distinct(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{Columns: cols(scalar("team")), Distinct: true})
	if res.Count != 2 {
		t.Fatalf("want 2 distinct teams, got %d", res.Count)
	}
}

func TestExecute_NumericSortOrdersByValueNotLex(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{
		Columns: cols(tcol("apps", 1)),
		OrderBy: []Sort{{Column: 0, Numeric: true}},
	})
	// numeric: 10,40,100,200 - lexical would give 10,100,200,40.
	got := []string{res.Rows[0][0].Text, res.Rows[1][0].Text, res.Rows[2][0].Text, res.Rows[3][0].Text}
	want := []string{"10", "40", "100", "200"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("numeric sort wrong: got %v want %v", got, want)
		}
	}
}

func TestExecute_GroupCountAndCountDistinct(t *testing.T) {
	m := sampleMatrix()
	res, err := m.Execute(Spec{
		Columns:  cols(scalar("team"), tcol("apps", 1)),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "count", Header: "rows"}, {Func: "count_distinct", Header: "forms"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 2 {
		t.Fatalf("want 2 groups, got %d", res.Count)
	}
	// Each team has 2 rows but 1 distinct form.
	for _, r := range res.Rows {
		if r[1].Text != "2" {
			t.Fatalf("count rows: want 2, got %s", r[1].Text)
		}
		if r[2].Text != "1" {
			t.Fatalf("count_distinct forms: want 1, got %s", r[2].Text)
		}
	}
}

func TestExecute_GroupSumAvgMinMax(t *testing.T) {
	m := sampleMatrix()
	res, err := m.Execute(Spec{
		Columns: cols(scalar("team"), tcol("apps", 1)),
		GroupBy: []int{0},
		Measures: []Measure{
			{Func: "sum", Source: tcol("apps", 1), Header: "sum"},
			{Func: "avg", Source: tcol("apps", 1), Header: "avg"},
			{Func: "min", Source: tcol("apps", 1), Header: "min"},
			{Func: "max", Source: tcol("apps", 1), Header: "max"},
		},
		OrderBy: []Sort{{Column: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Alpha: 100,40 -> sum 140 avg 70 min 40 max 100
	a := res.Rows[0]
	if a[1].Text != "140" || a[2].Text != "70" || a[3].Text != "40" || a[4].Text != "100" {
		t.Fatalf("Alpha aggregates wrong: %v", []string{a[1].Text, a[2].Text, a[3].Text, a[4].Text})
	}
	// Beta: 10,200 -> sum 210 avg 105 min 10 max 200
	b := res.Rows[1]
	if b[1].Text != "210" || b[2].Text != "105" || b[3].Text != "10" || b[4].Text != "200" {
		t.Fatalf("Beta aggregates wrong: %v", []string{b[1].Text, b[2].Text, b[3].Text, b[4].Text})
	}
}

func TestExecute_TypedColumnAnomalySurfaced(t *testing.T) {
	// A number-hinted column with a non-numeric cell. Input enforces types,
	// so this is corruption: it must be surfaced, not silently dropped. The
	// aggregate still computes over the valid cells (10+5=15), and the bad
	// value is reported in Anomalies.
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("g"))}, {ID: sourceID(scalar("n")), Hint: "number"}},
		Rows: []MatrixRow{
			{Form: "a", Cells: []string{"x", "10"}},
			{Form: "b", Cells: []string{"x", "oops"}},
			{Form: "c", Cells: []string{"x", "5"}},
		},
	}
	res, _ := m.Execute(Spec{
		Columns:  cols(scalar("g")),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "sum", Source: scalar("n"), Header: "s"}},
	})
	if res.Rows[0][1].Text != "15" {
		t.Fatalf("want 15 over valid cells, got %s", res.Rows[0][1].Text)
	}
	if len(res.Anomalies) != 1 {
		t.Fatalf("want 1 surfaced anomaly, got %d", len(res.Anomalies))
	}
	a := res.Anomalies[0]
	if a.Form != "b" || a.Value != "oops" || a.Expected != "number" {
		t.Fatalf("anomaly wrong: %+v", a)
	}
}

func TestExecute_BlankInTypedColumnIsNotAnomaly(t *testing.T) {
	// A blank cell is an unfilled field, not a type violation.
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("n")), Hint: "number"}},
		Rows: []MatrixRow{{Form: "a", Cells: []string{""}}, {Form: "b", Cells: []string{"3"}}},
	}
	res, _ := m.Execute(Spec{Columns: cols(scalar("n"))})
	if len(res.Anomalies) != 0 {
		t.Fatalf("blank should not be an anomaly, got %d", len(res.Anomalies))
	}
}

func TestExecute_DateColumnAnomaly(t *testing.T) {
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("d")), Hint: "date"}},
		Rows: []MatrixRow{
			{Form: "a", Cells: []string{"2026-05-29"}},
			{Form: "b", Cells: []string{"not-a-date"}},
		},
	}
	res, _ := m.Execute(Spec{Columns: cols(scalar("d"))})
	if len(res.Anomalies) != 1 || res.Anomalies[0].Expected != "date" {
		t.Fatalf("want 1 date anomaly, got %+v", res.Anomalies)
	}
}

func TestExecute_AllFilterOps(t *testing.T) {
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("n")), Hint: "number"}},
		Rows: []MatrixRow{
			{Form: "1", Cells: []string{"1"}},
			{Form: "2", Cells: []string{"2"}},
			{Form: "3", Cells: []string{"3"}},
		},
	}
	cases := []struct {
		op   string
		val  string
		want int
	}{
		{"eq", "2", 1},
		{"ne", "2", 2},
		{"lt", "2", 1},
		{"le", "2", 2},
		{"gt", "2", 1},
		{"ge", "2", 2},
	}
	for _, c := range cases {
		res, err := m.Execute(Spec{
			Columns: cols(scalar("n")),
			Filters: []Filter{{Source: scalar("n"), Op: c.op, Value: c.val}},
		})
		if err != nil {
			t.Fatalf("%s: %v", c.op, err)
		}
		if res.Count != c.want {
			t.Fatalf("op %s value %s: want %d rows, got %d", c.op, c.val, c.want, res.Count)
		}
	}
}

func TestExecute_DescAndMultiKeySort(t *testing.T) {
	m := sampleMatrix()
	// Sort by team desc, then cost asc numeric.
	res, _ := m.Execute(Spec{
		Columns: cols(scalar("team"), tcol("apps", 1)),
		OrderBy: []Sort{{Column: 0, Desc: true}, {Column: 1, Numeric: true}},
	})
	// Beta rows first (desc), each block ordered by cost asc.
	if res.Rows[0][0].Text != "Beta" || res.Rows[0][1].Text != "10" {
		t.Fatalf("first row wrong: %v", []string{res.Rows[0][0].Text, res.Rows[0][1].Text})
	}
	if res.Rows[2][0].Text != "Alpha" || res.Rows[2][1].Text != "40" {
		t.Fatalf("third row wrong: %v", []string{res.Rows[2][0].Text, res.Rows[2][1].Text})
	}
}

func TestExecute_DistinctMultiColumnTuple(t *testing.T) {
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("a"))}, {ID: sourceID(scalar("b"))}},
		Rows: []MatrixRow{
			{Form: "1", Cells: []string{"x", "1"}},
			{Form: "2", Cells: []string{"x", "1"}},
			{Form: "3", Cells: []string{"x", "2"}},
		},
	}
	res, _ := m.Execute(Spec{Columns: cols(scalar("a"), scalar("b")), Distinct: true})
	if res.Count != 2 {
		t.Fatalf("want 2 distinct (x,1),(x,2), got %d", res.Count)
	}
}

func TestExecute_MultiDimGroup(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{
		Columns:  cols(scalar("team"), facet("tier"), tcol("apps", 0)),
		GroupBy:  []int{0, 1},
		Measures: []Measure{{Func: "count", Header: "c"}},
		OrderBy:  []Sort{{Column: 0}},
	})
	// team x tier: (Alpha,gold)=2, (Beta,silver)=2.
	if res.Count != 2 {
		t.Fatalf("want 2 (team,tier) groups, got %d", res.Count)
	}
	if len(res.Columns) != 3 {
		t.Fatalf("want 3 columns (team, tier, c), got %d", len(res.Columns))
	}
}

// crossMatrix is one form with table A (apps: 2 rows, with cost) cartesian
// x table B (tasks: 3 rows). 6 exploded rows. Provenance lets a sum over
// A's cost count each A-row once despite B's 3x fan.
func crossMatrix() *Matrix {
	cols := []MatrixCol{
		{ID: sourceID(tcol("apps", 0))},
		{ID: sourceID(tcol("apps", 1)), Hint: "number"},
		{ID: sourceID(tcol("tasks", 0))},
	}
	appNames := []string{"billing", "search"}
	appCost := []string{"100", "40"}
	tasks := []string{"t1", "t2", "t3"}
	var rows []MatrixRow
	for ai := range 2 {
		for bi := range 3 {
			rows = append(rows, MatrixRow{
				Form: "f.json",
				Origins: []Origin{
					{Field: "apps", Row: ai, Count: 2},
					{Field: "tasks", Row: bi, Count: 3},
				},
				Cells: []string{appNames[ai], appCost[ai], tasks[bi]},
			})
		}
	}
	return &Matrix{Cols: cols, Rows: rows}
}

func TestExecute_CartesianKeepsProvenanceAndSumStaysHonest(t *testing.T) {
	m := crossMatrix()
	// Listing is the full cartesian: 2 apps x 3 tasks = 6 rows.
	list, _ := m.Execute(Spec{Columns: cols(tcol("apps", 0), tcol("tasks", 0))})
	if list.Count != 6 {
		t.Fatalf("want 6 cartesian rows, got %d", list.Count)
	}
	// Sum of apps.cost grouped by task: each app row counts once per group
	// (100+40=140), NOT inflated by the cartesian fan (which would give 420).
	gr, _ := m.Execute(Spec{
		Columns:  cols(tcol("tasks", 0), tcol("apps", 1)),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "sum", Source: tcol("apps", 1), Header: "s"}},
		OrderBy:  []Sort{{Column: 0}},
	})
	// Grouped by task: each task pairs with both apps once, so sum=140 per
	// task group, not 140 plus duplicate inflation.
	for _, r := range gr.Rows {
		if r[1].Text != "140" {
			t.Fatalf("task %s sum=%s, want 140 (apps counted once)", r[0].Text, r[1].Text)
		}
	}
}

func TestExecute_EmptyMatrix(t *testing.T) {
	m := &Matrix{Cols: []MatrixCol{{ID: sourceID(scalar("a"))}}}
	res, err := m.Execute(Spec{Columns: cols(scalar("a"))})
	if err != nil || res.Count != 0 {
		t.Fatalf("empty matrix: err=%v count=%d", err, res.Count)
	}
}

func TestExecute_Limit(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{Columns: cols(tcol("apps", 0)), Limit: 2})
	if res.Count != 2 {
		t.Fatalf("want 2 (limited), got %d", res.Count)
	}
}

func TestExecute_UnknownSourceErrors(t *testing.T) {
	m := sampleMatrix()
	if _, err := m.Execute(Spec{Columns: cols(scalar("nope"))}); err == nil {
		t.Fatal("want error for unknown projected source")
	}
}

func TestExecute_FilterOnUnknownSourceDropsAll(t *testing.T) {
	m := sampleMatrix()
	res, err := m.Execute(Spec{
		Columns: cols(scalar("team")),
		Filters: []Filter{{Source: scalar("ghost"), Op: "eq", Value: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 rows (unsatisfiable filter), got %d", res.Count)
	}
}

func TestExecute_GroupColumnOutOfRange(t *testing.T) {
	m := sampleMatrix()
	if _, err := m.Execute(Spec{Columns: cols(scalar("team")), GroupBy: []int{5}}); err == nil {
		t.Fatal("want error for group index out of range")
	}
}

func TestExecute_OrderColumnOutOfRange(t *testing.T) {
	m := sampleMatrix()
	if _, err := m.Execute(Spec{Columns: cols(scalar("team")), OrderBy: []Sort{{Column: 9}}}); err == nil {
		t.Fatal("want error for order index out of range")
	}
}

func TestExecute_FacetGroup(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{
		Columns:  cols(facet("tier")),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "count_distinct", Header: "forms"}},
		OrderBy:  []Sort{{Column: 0}},
	})
	if res.Count != 2 {
		t.Fatalf("want 2 tiers, got %d", res.Count)
	}
}
