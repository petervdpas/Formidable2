package query

import (
	"errors"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// errTwoRows is a sentinel a concurrency goroutine reports when the group
// result row count is not the expected 2.
var errTwoRows = errors.New("want 2 group rows")

// numMatrix is a single-column number-hinted matrix with mixed cells:
// numeric, blank, and non-numeric. Used to pin coercion-drop behavior.
func numMatrix() *Matrix {
	return &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("n")), Hint: "number"}},
		Rows: []MatrixRow{
			{Form: "a", Cells: []string{"10"}},
			{Form: "b", Cells: []string{""}},
			{Form: "c", Cells: []string{"oops"}},
			{Form: "d", Cells: []string{"5"}},
		},
	}
}

func TestExecute_NumericFilterDropsBlankAndNonNumeric(t *testing.T) {
	m := numMatrix()
	ops := []struct {
		op   string
		val  string
		want int
	}{
		{"lt", "100", 2}, // 10,5 pass; blank+oops drop
		{"le", "10", 2},  // 10,5 pass
		{"gt", "0", 2},   // 10,5 pass
		{"ge", "5", 2},   // 10,5 pass
	}
	for _, c := range ops {
		res, err := m.Execute(Spec{
			Columns: cols(scalar("n")),
			Filters: []Filter{{Source: scalar("n"), Op: c.op, Value: c.val}},
		})
		if err != nil {
			t.Fatalf("op %s: %v", c.op, err)
		}
		if res.Count != c.want {
			t.Fatalf("op %s val %s: want %d surviving rows, got %d", c.op, c.val, c.want, res.Count)
		}
	}
}

func TestExecute_NumericFilterBadComparandDropsAll(t *testing.T) {
	m := numMatrix()
	res, err := m.Execute(Spec{
		Columns: cols(scalar("n")),
		Filters: []Filter{{Source: scalar("n"), Op: "lt", Value: "notnum"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("non-numeric comparand drops every row, got %d", res.Count)
	}
}

func TestExecute_NumericSortUnparseableSortLast(t *testing.T) {
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("n"))}},
		Rows: []MatrixRow{
			{Form: "1", Cells: []string{"oops"}},
			{Form: "2", Cells: []string{"40"}},
			{Form: "3", Cells: []string{""}},
			{Form: "4", Cells: []string{"5"}},
		},
	}
	res, err := m.Execute(Spec{
		Columns: cols(scalar("n")),
		OrderBy: []Sort{{Column: 0, Numeric: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 4 {
		t.Fatalf("want all 4 rows after sort, got %d", res.Count)
	}
	// Parseable ascending first (5,40), then the two unparseable in stable
	// input order (oops then blank).
	got := []string{res.Rows[0][0].Text, res.Rows[1][0].Text, res.Rows[2][0].Text, res.Rows[3][0].Text}
	want := []string{"5", "40", "oops", ""}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("numeric sort with unparseable: got %v want %v", got, want)
		}
	}
}

func TestExecute_AggregatesOverAllBlankGroupAreEmpty(t *testing.T) {
	// One group, every measured cell blank or non-numeric: no NaN, no panic,
	// empty text per aggregate.
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("g"))}, {ID: sourceID(scalar("n")), Hint: "number"}},
		Rows: []MatrixRow{
			{Form: "a", Cells: []string{"grp", ""}},
			{Form: "b", Cells: []string{"grp", "  "}},
			{Form: "c", Cells: []string{"grp", "x"}},
		},
	}
	res, err := m.Execute(Spec{
		Columns: cols(scalar("g")),
		GroupBy: []int{0},
		Measures: []Measure{
			{Func: "sum", Source: scalar("n"), Header: "s"},
			{Func: "avg", Source: scalar("n"), Header: "a"},
			{Func: "min", Source: scalar("n"), Header: "mn"},
			{Func: "max", Source: scalar("n"), Header: "mx"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 {
		t.Fatalf("want 1 group, got %d", res.Count)
	}
	r := res.Rows[0]
	for i, name := range []string{"sum", "avg", "min", "max"} {
		if r[i+1].Text != "" || r[i+1].Num != nil {
			t.Fatalf("%s over all-blank should be empty text/nil num, got %q num=%v", name, r[i+1].Text, r[i+1].Num)
		}
	}
}

func TestExecute_CountDistinctCountsFormsNotRows(t *testing.T) {
	// A cartesian fan: 1 form, 2 apps x 3 tasks = 6 rows. count_distinct of
	// the group must be 1 (one form), not 6 (rows) even though dedupe would
	// inflate to 6 if it keyed on rows.
	m := crossMatrix()
	res, err := m.Execute(Spec{
		Columns:  cols(tcol("apps", 0)),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "count", Header: "rows"}, {Func: "count_distinct", Header: "forms"}},
		OrderBy:  []Sort{{Column: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 2 {
		t.Fatalf("want 2 app groups, got %d", res.Count)
	}
	// Two app names, each fanned by 3 tasks -> 3 rows per group, 1 form each.
	for _, r := range res.Rows {
		if r[1].Text != "3" {
			t.Fatalf("app %s row count: want 3, got %s", r[0].Text, r[1].Text)
		}
		if r[2].Text != "1" {
			t.Fatalf("app %s count_distinct forms: want 1, got %s", r[0].Text, r[2].Text)
		}
	}
}

func TestExecute_UnknownFilterOpDropsAll(t *testing.T) {
	// passOne returns false for any op outside eq/ne/lt/le/gt/ge, so an
	// unknown op is an unsatisfiable filter that drops every row.
	m := numMatrix()
	res, err := m.Execute(Spec{
		Columns: cols(scalar("n")),
		Filters: []Filter{{Source: scalar("n"), Op: "contains", Value: "1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("unknown op should drop all rows, got %d", res.Count)
	}
}

func TestExecute_EmptyFilterOpDropsAll(t *testing.T) {
	m := numMatrix()
	res, _ := m.Execute(Spec{
		Columns: cols(scalar("n")),
		Filters: []Filter{{Source: scalar("n"), Op: "", Value: "10"}},
	})
	if res.Count != 0 {
		t.Fatalf("empty op should drop all rows, got %d", res.Count)
	}
}

func TestExecute_NegativeGroupIndexErrors(t *testing.T) {
	m := sampleMatrix()
	_, err := m.Execute(Spec{Columns: cols(scalar("team")), GroupBy: []int{-1}})
	if err == nil {
		t.Fatal("want error for negative group index")
	}
	if err.Error() != "query: group column -1 out of range" {
		t.Fatalf("unexpected group error: %v", err)
	}
}

func TestExecute_GroupIndexOutOfRangeExactError(t *testing.T) {
	m := sampleMatrix()
	_, err := m.Execute(Spec{Columns: cols(scalar("team")), GroupBy: []int{7}})
	if err == nil || err.Error() != "query: group column 7 out of range" {
		t.Fatalf("want exact group range error, got %v", err)
	}
}

func TestExecute_OrderIndexOutOfRangeExactError(t *testing.T) {
	m := sampleMatrix()
	_, err := m.Execute(Spec{Columns: cols(scalar("team")), OrderBy: []Sort{{Column: 9}}})
	if err == nil || err.Error() != "query: order column 9 out of range" {
		t.Fatalf("want exact order range error, got %v", err)
	}
}

func TestExecute_NegativeOrderIndexErrors(t *testing.T) {
	m := sampleMatrix()
	_, err := m.Execute(Spec{Columns: cols(scalar("team")), OrderBy: []Sort{{Column: -3}}})
	if err == nil || err.Error() != "query: order column -3 out of range" {
		t.Fatalf("want exact negative order range error, got %v", err)
	}
}

func TestExecute_GroupOrderRangeCheckedAgainstResultColumns(t *testing.T) {
	// In group mode, order is checked against the result column count (dims +
	// measures), which is 2 here. Column 2 is out of range and errors.
	m := sampleMatrix()
	_, err := m.Execute(Spec{
		Columns:  cols(scalar("team")),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "count", Header: "c"}},
		OrderBy:  []Sort{{Column: 2}},
	})
	if err == nil || err.Error() != "query: order column 2 out of range" {
		t.Fatalf("want order range error against 2 result columns, got %v", err)
	}
}

func TestExecute_DistinctMultiColumnTupleCollapsesDuplicates(t *testing.T) {
	// Three rows, two share the full (a,b) tuple. Distinct collapses to 2.
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
		t.Fatalf("distinct (x,1),(x,2) want 2, got %d", res.Count)
	}
	// First survivor keeps the first occurrence, second is (x,2).
	if res.Rows[0][1].Text != "1" || res.Rows[1][1].Text != "2" {
		t.Fatalf("distinct survivors wrong: %v / %v", res.Rows[0], res.Rows[1])
	}
}

func TestExecute_AvgSingleValueNoDivByZero(t *testing.T) {
	// One numeric cell in the group: avg = that value, n=1, no div-by-zero.
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("g"))}, {ID: sourceID(scalar("n")), Hint: "number"}},
		Rows: []MatrixRow{
			{Form: "a", Cells: []string{"g", "7"}},
			{Form: "b", Cells: []string{"g", ""}},
		},
	}
	res, err := m.Execute(Spec{
		Columns:  cols(scalar("g")),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "avg", Source: scalar("n"), Header: "a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 {
		t.Fatalf("want 1 group, got %d", res.Count)
	}
	c := res.Rows[0][1]
	if c.Text != "7" || c.Num == nil || *c.Num != 7 {
		t.Fatalf("avg over single value want 7 with num=7, got %q num=%v", c.Text, c.Num)
	}
}

func TestExecute_LimitGreaterThanRowCountReturnsAll(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{Columns: cols(tcol("apps", 0)), Limit: 99})
	if res.Count != 4 {
		t.Fatalf("limit beyond rowcount returns all 4, got %d", res.Count)
	}
}

func TestExecute_LimitZeroReturnsAll(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{Columns: cols(tcol("apps", 0)), Limit: 0})
	if res.Count != 4 {
		t.Fatalf("limit 0 means no limit, want 4, got %d", res.Count)
	}
}

func TestExecute_NegativeLimitReturnsAll(t *testing.T) {
	m := sampleMatrix()
	res, _ := m.Execute(Spec{Columns: cols(tcol("apps", 0)), Limit: -5})
	if res.Count != 4 {
		t.Fatalf("negative limit treated as no limit, want 4, got %d", res.Count)
	}
}

// facetTpl is a template carrying a single facet "tier" so Prepare accepts
// a facet source.
func facetTpl() *template.Template {
	return &template.Template{
		Facets: []template.Facet{{Key: "tier"}},
		Fields: []template.Field{{Key: "team", Type: "text"}},
	}
}

func TestPrepare_FacetUnsetVsSetEmptyBothBlank(t *testing.T) {
	// Boundary: a form with no facet selection and a form with the facet key
	// present but empty both flatten to a blank facet cell.
	l := fakeLoader{
		tpl: facetTpl(),
		forms: []FormData{
			{Filename: "unset.json", Data: map[string]any{"team": "A"}, Facets: map[string]string{}},
			{Filename: "empty.json", Data: map[string]any{"team": "B"}, Facets: map[string]string{"tier": ""}},
			{Filename: "set.json", Data: map[string]any{"team": "C"}, Facets: map[string]string{"tier": "gold"}},
		},
	}
	spec := Spec{Template: "t.yaml", Columns: cols(facet("tier"))}
	m, err := Prepare(spec, l)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := m.Execute(spec)
	if res.Count != 3 {
		t.Fatalf("want 3 rows, got %d", res.Count)
	}
	blanks := 0
	for _, r := range res.Rows {
		if r[0].Text == "" {
			blanks++
		}
	}
	if blanks != 2 {
		t.Fatalf("unset and set-empty should both be blank: want 2 blanks, got %d", blanks)
	}
	// Filtering for blank facet (eq "") catches exactly the two boundary rows.
	fres, _ := m.Execute(Spec{
		Columns: cols(facet("tier")),
		Filters: []Filter{{Source: facet("tier"), Op: "eq", Value: ""}},
	})
	if fres.Count != 2 {
		t.Fatalf("eq blank facet should match unset+empty, want 2, got %d", fres.Count)
	}
}

func TestPrepare_UnknownFacetErrors(t *testing.T) {
	l := fakeLoader{tpl: facetTpl()}
	_, err := Prepare(Spec{Template: "t.yaml", Columns: cols(facet("ghost"))}, l)
	if err == nil || err.Error() != `query: unknown facet "ghost"` {
		t.Fatalf("want unknown facet error, got %v", err)
	}
}

func TestPrepare_UnknownFieldErrors(t *testing.T) {
	l := fakeLoader{tpl: appsTpl()}
	_, err := Prepare(Spec{Template: "t.yaml", Columns: cols(scalar("nope"))}, l)
	if err == nil || err.Error() != `query: unknown field "nope"` {
		t.Fatalf("want unknown field error, got %v", err)
	}
}

func TestExecute_ColumnSourceUnknownExactError(t *testing.T) {
	m := sampleMatrix()
	_, err := m.Execute(Spec{Columns: cols(scalar("ghost"))})
	if err == nil || err.Error() != "query: source not available in matrix: field|ghost|" {
		t.Fatalf("want exact unknown-source error, got %v", err)
	}
}

func TestExecute_MeasureSourceUnknownInGroupErrors(t *testing.T) {
	// A sum measure whose source is not a matrix column errors out of group.
	m := sampleMatrix()
	_, err := m.Execute(Spec{
		Columns:  cols(scalar("team")),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "sum", Source: scalar("ghost"), Header: "s"}},
	})
	if err == nil || err.Error() != "query: source not available in matrix: field|ghost|" {
		t.Fatalf("want unknown measure-source error, got %v", err)
	}
}

func TestExecute_AvgDividesOnlyByCoercibleCells(t *testing.T) {
	// Group of 3 rows, one cell non-numeric: avg divides by the 2 valid cells
	// (10+5)/2 = 7.5, not by 3. The bad cell is dropped, not counted as zero.
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("g"))}, {ID: sourceID(scalar("n")), Hint: "number"}},
		Rows: []MatrixRow{
			{Form: "a", Cells: []string{"g", "10"}},
			{Form: "b", Cells: []string{"g", "bad"}},
			{Form: "c", Cells: []string{"g", "5"}},
		},
	}
	res, err := m.Execute(Spec{
		Columns:  cols(scalar("g")),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "avg", Source: scalar("n"), Header: "a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := res.Rows[0][1]
	if c.Text != "7.5" || c.Num == nil || *c.Num != 7.5 {
		t.Fatalf("avg over coercible cells want 7.5, got %q num=%v", c.Text, c.Num)
	}
}

func TestExecute_CountIncludesBlankAndNonNumericRows(t *testing.T) {
	// count is a row tally, independent of coercion: a group of 3 rows with
	// two uncoercible measure cells still counts 3.
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("g"))}, {ID: sourceID(scalar("n")), Hint: "number"}},
		Rows: []MatrixRow{
			{Form: "a", Cells: []string{"g", ""}},
			{Form: "b", Cells: []string{"g", "x"}},
			{Form: "c", Cells: []string{"g", "9"}},
		},
	}
	res, err := m.Execute(Spec{
		Columns: cols(scalar("g")),
		GroupBy: []int{0},
		Measures: []Measure{
			{Func: "count", Header: "c"},
			{Func: "sum", Source: scalar("n"), Header: "s"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0][1].Text != "3" {
		t.Fatalf("count want 3, got %s", res.Rows[0][1].Text)
	}
	if res.Rows[0][2].Text != "9" {
		t.Fatalf("sum over the lone numeric cell want 9, got %s", res.Rows[0][2].Text)
	}
}

func TestExecute_CountDistinctTwoFormsNotInflatedByFan(t *testing.T) {
	// Two forms in one group, each fanned to 2 rows. count is 4 rows but
	// count_distinct is 2 forms.
	m := &Matrix{
		Cols: []MatrixCol{{ID: sourceID(scalar("g"))}},
		Rows: []MatrixRow{
			{Form: "a.json", Cells: []string{"g"}},
			{Form: "a.json", Cells: []string{"g"}},
			{Form: "b.json", Cells: []string{"g"}},
			{Form: "b.json", Cells: []string{"g"}},
		},
	}
	res, err := m.Execute(Spec{
		Columns: cols(scalar("g")),
		GroupBy: []int{0},
		Measures: []Measure{
			{Func: "count", Header: "rows"},
			{Func: "count_distinct", Header: "forms"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0][1].Text != "4" {
		t.Fatalf("row count want 4, got %s", res.Rows[0][1].Text)
	}
	if res.Rows[0][2].Text != "2" {
		t.Fatalf("count_distinct forms want 2, got %s", res.Rows[0][2].Text)
	}
}

func TestExecute_LimitEqualsRowCountReturnsAll(t *testing.T) {
	m := sampleMatrix()
	res, err := m.Execute(Spec{Columns: cols(tcol("apps", 0)), Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 4 {
		t.Fatalf("limit == rowcount returns all 4, got %d", res.Count)
	}
}

func TestExecute_LimitOneTruncatesToFirst(t *testing.T) {
	m := sampleMatrix()
	res, err := m.Execute(Spec{
		Columns: cols(tcol("apps", 0)),
		OrderBy: []Sort{{Column: 0}},
		Limit:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 {
		t.Fatalf("limit 1 keeps 1 row, got %d", res.Count)
	}
	// Sorted apps names: billing, billing, report, search. First is billing.
	if res.Rows[0][0].Text != "billing" {
		t.Fatalf("limit 1 after sort keeps billing, got %s", res.Rows[0][0].Text)
	}
}

func TestExecute_NeFilterKeepsBlankAndMismatch(t *testing.T) {
	// ne is a pure string compare: blank != "x" is true, so a blank cell
	// survives an ne filter (unlike the numeric ops which drop blanks).
	m := numMatrix()
	res, err := m.Execute(Spec{
		Columns: cols(scalar("n")),
		Filters: []Filter{{Source: scalar("n"), Op: "ne", Value: "10"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 4 rows: 10 (dropped), blank, oops, 5 -> 3 survive.
	if res.Count != 3 {
		t.Fatalf("ne 10 over [10,blank,oops,5] want 3, got %d", res.Count)
	}
}

func TestExecute_EqBlankMatchesOnlyBlankCells(t *testing.T) {
	m := numMatrix()
	res, err := m.Execute(Spec{
		Columns: cols(scalar("n")),
		Filters: []Filter{{Source: scalar("n"), Op: "eq", Value: ""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Only the one blank cell matches eq "".
	if res.Count != 1 {
		t.Fatalf("eq blank want 1 blank cell, got %d", res.Count)
	}
}

func TestExecute_ConcurrentReadsAreStable(t *testing.T) {
	// Execute must not mutate the shared Matrix: concurrent group queries from
	// many goroutines must each see the same exact result and race-clean.
	m := sampleMatrix()
	spec := Spec{
		Columns: cols(scalar("team"), tcol("apps", 1)),
		GroupBy: []int{0},
		Measures: []Measure{
			{Func: "sum", Source: tcol("apps", 1), Header: "s"},
		},
		OrderBy: []Sort{{Column: 0}},
	}
	const goroutines = 32
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	sums := make([][2]string, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			res, err := m.Execute(spec)
			if err != nil {
				errs[g] = err
				return
			}
			if len(res.Rows) != 2 {
				errs[g] = errTwoRows
				return
			}
			sums[g] = [2]string{res.Rows[0][1].Text, res.Rows[1][1].Text}
		}(g)
	}
	wg.Wait()
	for g := 0; g < goroutines; g++ {
		if errs[g] != nil {
			t.Fatalf("goroutine %d: %v", g, errs[g])
		}
		// Alpha sum 140 (100+40), Beta sum 210 (10+200).
		if sums[g][0] != "140" || sums[g][1] != "210" {
			t.Fatalf("goroutine %d unstable sums: %v", g, sums[g])
		}
	}
}
