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

// seedFanDB builds a template whose forms carry a multi-row table column
// "apps" (col0) plus a scalar "status", so tests can exercise filter
// folding against a fanned column. Form a has two app rows, b one, c none.
func seedFanDB(t *testing.T) *Manager {
	t.Helper()
	db := openTestDB(t)
	mk := func(file, status string, apps ...string) FormRow {
		vals := []FormValueRow{{FieldKey: "status", ValueType: "text", Text: status}}
		for _, a := range apps {
			col := 0
			vals = append(vals, FormValueRow{FieldKey: "apps", Col: &col, ValueType: "text", Text: a})
		}
		return FormRow{Template: "t.yaml", Filename: file, Mtime: 100, Values: vals}
	}
	must(t, Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("t", 100)},
		UpsertForms: []FormRow{
			mk("a.meta.json", "x", "alpha", "beta"),
			mk("b.meta.json", "y", "alpha"),
			mk("c.meta.json", "z"), // no apps at all
		},
	}))
	return &Manager{db: db}
}

// The core fix: a filter on a column you also project must align with the
// projection, not match at the form level. Filtering apps=alpha while
// projecting apps must drop form a's "beta" cell, not return it because
// the form happens to also contain an "alpha".
func TestProjectRows_FilterFoldedIntoFannedColumn(t *testing.T) {
	m := seedFanDB(t)
	c0 := 0
	rows, err := m.ProjectRows("t.yaml", ProjectSpec{
		Cols: []ProjectCol{
			{Kind: "field", Key: "status"},
			{Kind: "field", Key: "apps", Col: &c0},
		},
		Filters: []AggFilter{{Kind: "field", Key: "apps", Col: &c0, Op: "eq", Value: "alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (a:alpha, b:alpha) - the beta cell must not survive", len(rows))
	}
	for _, r := range rows {
		if r.Cells[1].Text != "alpha" {
			t.Errorf("fanned cell = %q, want alpha (filter must align with projection)", r.Cells[1].Text)
		}
	}
}

// A filter folds with ne too, and the form with no apps at all drops out
// (the folded join is INNER).
func TestProjectRows_FoldedNeExcludesAndDropsEmpty(t *testing.T) {
	m := seedFanDB(t)
	c0 := 0
	rows, err := m.ProjectRows("t.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "apps", Col: &c0}},
		Filters: []AggFilter{{Kind: "field", Key: "apps", Col: &c0, Op: "ne", Value: "alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Cells[0].Text != "beta" {
		t.Fatalf("got %+v, want exactly one row (beta)", rows)
	}
}

// A filter on a column that is NOT projected still scopes at the form
// level: project status only, filter apps=beta -> only form a qualifies,
// and status is not fanned.
func TestProjectRows_FilterOnUnprojectedColumnScopesByForm(t *testing.T) {
	m := seedFanDB(t)
	c0 := 0
	rows, err := m.ProjectRows("t.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "status"}},
		Filters: []AggFilter{{Kind: "field", Key: "apps", Col: &c0, Op: "eq", Value: "beta"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Cells[0].Text != "x" {
		t.Fatalf("got %+v, want one row status=x (form-level scope, no fan)", rows)
	}
}

// A folded facet filter restricts the projected facet to the matching
// option rather than scoping by form.
func TestProjectRows_FoldedFacetFilter(t *testing.T) {
	m := seedValuesDB(t)
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "facet", Key: "prio"}},
		Filters: []AggFilter{{Kind: "facet", Key: "prio", Op: "eq", Value: "high"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 high", len(rows))
	}
	for _, r := range rows {
		if r.Cells[0].Text != "high" {
			t.Errorf("facet cell = %q, want high", r.Cells[0].Text)
		}
	}
}

// A bad numeric value on a FOLDED filter (one targeting a projected
// column) must surface the same error as the un-folded path, not silently
// pass or panic.
func TestProjectRows_FoldedBadNumericFilter(t *testing.T) {
	m := seedFanDB(t)
	c0 := 0
	_, err := m.ProjectRows("t.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "apps", Col: &c0}},
		Filters: []AggFilter{{Kind: "field", Key: "apps", Col: &c0, Op: "gt", Value: "notanumber"}},
	})
	if err == nil {
		t.Fatal("expected error for non-numeric value on a folded filter")
	}
}

// sameSource must distinguish scalar from table-column of the same field:
// a filter on the scalar must NOT fold into a table-column projection of
// the same key (different col), it scopes by form instead.
func TestProjectRows_ScalarFilterDoesNotFoldIntoTableColumn(t *testing.T) {
	m := seedValuesDB(t)
	c0 := 0
	// items col0 is "row-<file>"; filter the scalar "status" (no col).
	// Projecting items.col0 with a status filter must scope by form, not
	// fold (mismatched col), so each matching form's single items cell
	// shows. status=high matches a and c -> 2 rows.
	rows, err := m.ProjectRows("basic.yaml", ProjectSpec{
		Cols:    []ProjectCol{{Kind: "field", Key: "items", Col: &c0}},
		Filters: []AggFilter{{Kind: "field", Key: "status", Op: "eq", Value: "high"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (forms a,c with status=high)", len(rows))
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

// FilterOps must stay in lockstep with what filterJoins accepts: every
// published op resolves, and an op outside the set is rejected.
func TestFilterOps_AllAcceptedNoDrift(t *testing.T) {
	for _, op := range FilterOps {
		if _, _, err := filterJoins([]AggFilter{{Kind: "field", Key: "x", Op: op, Value: "1"}}); err != nil {
			t.Errorf("published op %q rejected by filterJoins: %v", op, err)
		}
	}
	if _, _, err := filterJoins([]AggFilter{{Kind: "field", Key: "x", Op: "nope", Value: "1"}}); err == nil {
		t.Error("filterJoins accepted an op not in FilterOps")
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
