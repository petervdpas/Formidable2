package stat

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

func nf(v float64) sql.NullFloat64 { return sql.NullFloat64{Float64: v, Valid: true} }

// findCell returns the cell at the given coords, or nil.
func findCell(g *Grid, coords ...int) *GridCell {
	for i := range g.Cells {
		if reflect.DeepEqual(g.Cells[i].Coords, coords) {
			return &g.Cells[i]
		}
	}
	return nil
}

func TestEvaluate_Rank1_CountAndAvg(t *testing.T) {
	idx := &fakeIndex{
		total: 3,
		raw: []index.StatRawRow{
			{Dims: []string{"high"}, Nums: []sql.NullFloat64{nf(10)}},
			{Dims: []string{"low"}, Nums: []sql.NullFloat64{nf(20)}},
			{Dims: []string{"high"}, Nums: []sql.NullFloat64{nf(30)}},
		},
	}
	m := NewManager(idx)
	cfg := StatConfig{
		Measures: []Measure{
			{Op: OpCount},
			{Op: OpAvg, Source: &SourceRef{Kind: SourceField, Key: "amount"}},
		},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
	}
	g, err := m.Evaluate("t", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if g.Total != 3 {
		t.Errorf("total = %d, want 3", g.Total)
	}
	if !reflect.DeepEqual(g.Measures, []string{"count", "avg(amount)"}) {
		t.Errorf("measures = %v", g.Measures)
	}
	if len(g.Axes) != 1 || !reflect.DeepEqual(g.Axes[0].Labels, []string{"high", "low"}) {
		t.Fatalf("axes = %+v", g.Axes)
	}
	// high (coord 0): count 2, avg (10+30)/2 = 20
	if c := findCell(g, 0); c == nil || c.Values[0] != 2 || c.Values[1] != 20 {
		t.Errorf("high cell = %+v, want count2 avg20", c)
	}
	// low (coord 1): count 1, avg 20
	if c := findCell(g, 1); c == nil || c.Values[0] != 1 || c.Values[1] != 20 {
		t.Errorf("low cell = %+v, want count1 avg20", c)
	}
}

func TestEvaluate_AddsPercentShareOfMeasureTotal(t *testing.T) {
	idx := &fakeIndex{total: 4, raw: []index.StatRawRow{
		{Dims: []string{"a"}}, {Dims: []string{"a"}}, {Dims: []string{"a"}}, {Dims: []string{"b"}},
	}}
	g, err := NewManager(idx).Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "x"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// a = 3 of 4 mentions = 75%, b = 1 of 4 = 25% (share of the measure total,
	// not of grid.Total). Computed server-side, on every cell.
	if c := findCell(g, 0); c == nil || c.Pct[0] != 75 {
		t.Errorf("a pct = %+v, want 75", c)
	}
	if c := findCell(g, 1); c == nil || c.Pct[0] != 25 {
		t.Errorf("b pct = %+v, want 25", c)
	}
}

func TestEvaluate_PercentBaseForms_DividesByFormCount(t *testing.T) {
	// 3 mentions across 2 forms (total). pct forms: a = 2 of 2 forms = 100%.
	idx := &fakeIndex{total: 2, raw: []index.StatRawRow{
		{Dims: []string{"a"}}, {Dims: []string{"a"}}, {Dims: []string{"b"}},
	}}
	g, err := NewManager(idx).Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "x"}}},
		Percent:    PctForms,
	})
	if err != nil {
		t.Fatal(err)
	}
	// a = 2 mentions of 2 forms = 100%, b = 1 of 2 = 50% (vs 67/33 for distribution).
	if c := findCell(g, 0); c == nil || c.Pct[0] != 100 {
		t.Errorf("a pct(forms) = %+v, want 100", c)
	}
	if c := findCell(g, 1); c == nil || c.Pct[0] != 50 {
		t.Errorf("b pct(forms) = %+v, want 50", c)
	}
}

func TestEvaluate_PercentBaseNone_LeavesPctUnset(t *testing.T) {
	idx := &fakeIndex{total: 2, raw: []index.StatRawRow{{Dims: []string{"a"}}, {Dims: []string{"b"}}}}
	g, err := NewManager(idx).Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "x"}}},
		Percent:    PctNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c := findCell(g, 0); c == nil || c.Pct != nil {
		t.Errorf("pct should be unset for 'none', got %+v", c)
	}
}

func TestEvaluate_Rank0_Count(t *testing.T) {
	idx := &fakeIndex{
		total: 3,
		raw: []index.StatRawRow{
			{Dims: []string{}},
			{Dims: []string{}},
			{Dims: []string{}},
		},
	}
	m := NewManager(idx)
	g, err := m.Evaluate("t", StatConfig{Measures: []Measure{{Op: OpCount}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Axes) != 0 {
		t.Errorf("axes = %v, want none", g.Axes)
	}
	if len(g.Cells) != 1 || len(g.Cells[0].Coords) != 0 || g.Cells[0].Values[0] != 3 {
		t.Errorf("cells = %+v, want one scalar cell value 3", g.Cells)
	}
}

func TestEvaluate_Rank2_Crosstab(t *testing.T) {
	idx := &fakeIndex{
		total: 4,
		raw: []index.StatRawRow{
			{Dims: []string{"P1", "S1"}},
			{Dims: []string{"P1", "S2"}},
			{Dims: []string{"P2", "S1"}},
			{Dims: []string{"P1", "S1"}},
		},
	}
	m := NewManager(idx)
	cfg := StatConfig{
		Measures: []Measure{{Op: OpCount}},
		Dimensions: []Dimension{
			{Source: SourceRef{Kind: SourceFacet, Key: "prio"}},
			{Source: SourceRef{Kind: SourceFacet, Key: "stage"}},
		},
	}
	g, err := m.Evaluate("t", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g.Axes[0].Labels, []string{"P1", "P2"}) ||
		!reflect.DeepEqual(g.Axes[1].Labels, []string{"S1", "S2"}) {
		t.Fatalf("axes = %+v", g.Axes)
	}
	// (P1,S1) = 2
	if c := findCell(g, 0, 0); c == nil || c.Values[0] != 2 {
		t.Errorf("(P1,S1) = %+v, want 2", c)
	}
	// (P2,S2) never occurs -> no cell (sparse)
	if c := findCell(g, 1, 1); c != nil {
		t.Errorf("(P2,S2) should be absent, got %+v", c)
	}
}

func TestEvaluate_NullMeasureValuesExcluded(t *testing.T) {
	idx := &fakeIndex{
		total: 2,
		raw: []index.StatRawRow{
			{Dims: []string{"x"}, Nums: []sql.NullFloat64{nf(10)}},
			{Dims: []string{"x"}, Nums: []sql.NullFloat64{{Valid: false}}}, // no amount
		},
	}
	m := NewManager(idx)
	cfg := StatConfig{
		Measures: []Measure{
			{Op: OpCount},
			{Op: OpSum, Source: &SourceRef{Kind: SourceField, Key: "amount"}},
		},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "g"}}},
	}
	g, err := m.Evaluate("t", cfg)
	if err != nil {
		t.Fatal(err)
	}
	// count counts both forms; sum only the one with a value.
	if c := findCell(g, 0); c == nil || c.Values[0] != 2 || c.Values[1] != 10 {
		t.Errorf("cell = %+v, want count2 sum10", c)
	}
}

type fakeColResolver struct{ idx map[string]int }

func (f fakeColResolver) ColumnIndex(_, fieldKey, columnKey string) (int, bool) {
	v, ok := f.idx[fieldKey+"."+columnKey]
	return v, ok
}

func TestEvaluate_TableColumnDimension_CountsCells(t *testing.T) {
	// Three dataset cells across the forms: direct, via, direct.
	idx := &fakeIndex{
		total: 2,
		raw: []index.StatRawRow{
			{Dims: []string{"direct"}}, {Dims: []string{"via"}}, {Dims: []string{"direct"}},
		},
	}
	m := NewManager(idx)
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"grp.access": 1}})
	g, err := m.Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "access"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{}
	for ci, c := range g.Cells {
		want[g.Axes[0].Labels[g.Cells[ci].Coords[0]]] = c.Values[0]
	}
	if want["direct"] != 2 || want["via"] != 1 {
		t.Errorf("cell counts = %v, want direct:2 via:1", want)
	}
}

func TestEvaluate_ScalarCrossedWithTableColumn(t *testing.T) {
	// The cross-dimension case: count dataset cells per entity x access.
	// The table column fans out one row per cell; each is attributed to the
	// form's scalar (entity) value. Allowed by the guard (1 table source +
	// scalar dim + count).
	idx := &fakeIndex{
		total: 2,
		raw: []index.StatRawRow{
			{Dims: []string{"scm.A", "direct"}},
			{Dims: []string{"scm.A", "via"}},
			{Dims: []string{"scm.A", "direct"}},
			{Dims: []string{"scm.B", "via"}},
		},
	}
	m := NewManager(idx)
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"grp.access": 0}})
	g, err := m.Evaluate("t", StatConfig{
		Measures: []Measure{{Op: OpCount}},
		Dimensions: []Dimension{
			{Source: SourceRef{Kind: SourceField, Key: "entity"}},
			{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "access"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g.Axes[0].Labels, []string{"scm.A", "scm.B"}) ||
		!reflect.DeepEqual(g.Axes[1].Labels, []string{"direct", "via"}) {
		t.Fatalf("axes = %+v", g.Axes)
	}
	if c := findCell(g, 0, 0); c == nil || c.Values[0] != 2 { // scm.A x direct
		t.Errorf("(scm.A,direct) = %+v, want 2", c)
	}
	if c := findCell(g, 0, 1); c == nil || c.Values[0] != 1 { // scm.A x via
		t.Errorf("(scm.A,via) = %+v, want 1", c)
	}
	if c := findCell(g, 1, 1); c == nil || c.Values[0] != 1 { // scm.B x via
		t.Errorf("(scm.B,via) = %+v, want 1", c)
	}
	if c := findCell(g, 1, 0); c != nil { // scm.B x direct never occurs
		t.Errorf("(scm.B,direct) should be absent, got %+v", c)
	}
}

func TestEvaluate_Records_CountsDistinctFormsNotRows(t *testing.T) {
	// The SAMPLE case: form x lists QMU on two components (two rows), form y
	// lists it once. count() = 3 mentions; records() = 2 storage-items hit.
	idx := &fakeIndex{
		total: 2,
		raw: []index.StatRawRow{
			{Form: "x.meta.json", Dims: []string{"QMU"}},
			{Form: "x.meta.json", Dims: []string{"QMU"}},
			{Form: "y.meta.json", Dims: []string{"QMU"}},
			{Form: "x.meta.json", Dims: []string{"Bladework"}},
		},
	}
	m := NewManager(idx)
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})
	g, err := m.Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}, {Op: OpRecords}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g.Measures, []string{"count", "records"}) {
		t.Fatalf("measures = %v", g.Measures)
	}
	by := map[string][]float64{}
	for _, c := range g.Cells {
		by[g.Axes[0].Labels[c.Coords[0]]] = c.Values
	}
	// QMU: 3 mentions, 2 distinct storage-items.
	if got := by["QMU"]; len(got) != 2 || got[0] != 3 || got[1] != 2 {
		t.Errorf("QMU = %v, want [3 2] (count 3, records 2)", got)
	}
	// Bladework: 1 mention, 1 storage-item.
	if got := by["Bladework"]; len(got) != 2 || got[0] != 1 || got[1] != 1 {
		t.Errorf("Bladework = %v, want [1 1]", got)
	}
}

func TestEvaluate_Records_RanksByFirstMeasure(t *testing.T) {
	// count() first, records() second, top 2: the top-N is chosen by
	// mentions (count), the heaviness (records) rides along.
	idx := &fakeIndex{
		total: 4,
		raw: []index.StatRawRow{
			// A: 4 mentions across 2 forms.
			{Form: "f1", Dims: []string{"A"}}, {Form: "f1", Dims: []string{"A"}},
			{Form: "f2", Dims: []string{"A"}}, {Form: "f2", Dims: []string{"A"}},
			// B: 3 mentions across 3 forms.
			{Form: "f1", Dims: []string{"B"}}, {Form: "f2", Dims: []string{"B"}}, {Form: "f3", Dims: []string{"B"}},
			// C: 1 mention, 1 form (dropped by top 2).
			{Form: "f4", Dims: []string{"C"}},
		},
	}
	m := NewManager(idx)
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})
	g, err := m.Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}, {Op: OpRecords}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}, Top: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Ranked by count desc: A (4), B (3). C dropped.
	if want := []string{"A", "B"}; !reflect.DeepEqual(g.Axes[0].Labels, want) {
		t.Fatalf("labels = %v, want %v", g.Axes[0].Labels, want)
	}
	// A: count 4, records 2. B: count 3, records 3.
	if c := findCell(g, 0); c == nil || c.Values[0] != 4 || c.Values[1] != 2 {
		t.Errorf("A = %+v, want count4 records2", c)
	}
	if c := findCell(g, 1); c == nil || c.Values[0] != 3 || c.Values[1] != 3 {
		t.Errorf("B = %+v, want count3 records3", c)
	}
}

func TestEvaluate_RejectsNumericMeasureAlongsideRecordsWithTableDim(t *testing.T) {
	// records() is allowed on a table-column dimension, but a numeric
	// measure beside it would over-count (it repeats per fanned cell).
	m := NewManager(&fakeIndex{})
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})
	_, err := m.Evaluate("t", StatConfig{
		Measures: []Measure{
			{Op: OpRecords},
			{Op: OpAvg, Source: &SourceRef{Kind: SourceField, Key: "amount"}},
		},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}}},
	})
	if err == nil {
		t.Error("expected rejection: numeric measure alongside records() on a table-column dimension")
	}
}

func TestEvaluate_TableColumnNeedsResolver(t *testing.T) {
	m := NewManager(&fakeIndex{}) // no column resolver
	_, err := m.Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "access"}}},
	})
	if err == nil {
		t.Error("expected error when a table-column source has no resolver")
	}
}

func TestEvaluate_RejectsTwoTableColumnSources(t *testing.T) {
	m := NewManager(&fakeIndex{})
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"grp.access": 1, "grp.via": 2}})
	_, err := m.Evaluate("t", StatConfig{
		Measures: []Measure{{Op: OpCount}},
		Dimensions: []Dimension{
			{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "access"}},
			{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "via"}},
		},
	})
	if err == nil {
		t.Error("expected rejection of two table-column sources (fan-out)")
	}
}

func TestEvaluate_RejectsNumericMeasureWithTableDim(t *testing.T) {
	m := NewManager(&fakeIndex{})
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"grp.access": 1}})
	_, err := m.Evaluate("t", StatConfig{
		Measures: []Measure{
			{Op: OpCount},
			{Op: OpAvg, Source: &SourceRef{Kind: SourceField, Key: "amount"}},
		},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "access"}}},
	})
	if err == nil {
		t.Error("expected rejection: numeric measure alongside a table-column dimension")
	}
}

type fakeOptions struct{ labels map[string][]string }

func (f fakeOptions) DimensionLabels(_ string, src SourceRef) ([]CategoryOption, bool) {
	l, ok := f.labels[src.Key]
	if !ok {
		return nil, false
	}
	out := make([]CategoryOption, len(l))
	for i, v := range l {
		out[i] = CategoryOption{Value: v, Label: v}
	}
	return out, true
}

func TestEvaluate_FixedCategorySet_ShowsZeroCountInOrder(t *testing.T) {
	// Data only has SMALL and LARGE; the facet defines LARGE, MEDIUM, SMALL.
	idx := &fakeIndex{
		total: 3,
		raw: []index.StatRawRow{
			{Dims: []string{"SMALL"}},
			{Dims: []string{"LARGE"}},
			{Dims: []string{"SMALL"}},
		},
	}
	m := NewManager(idx)
	m.SetSourceOptions(fakeOptions{labels: map[string][]string{"tshirt": {"LARGE", "MEDIUM", "SMALL"}}})
	cfg := StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceFacet, Key: "tshirt"}}},
	}
	g, err := m.Evaluate("t", cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Defined order preserved, MEDIUM present despite zero rows.
	if want := []string{"LARGE", "MEDIUM", "SMALL"}; !reflect.DeepEqual(g.Axes[0].Labels, want) {
		t.Fatalf("labels = %v, want %v", g.Axes[0].Labels, want)
	}
	// LARGE (idx 0) = 1, SMALL (idx 2) = 2, MEDIUM (idx 1) has no cell.
	if c := findCell(g, 0); c == nil || c.Values[0] != 1 {
		t.Errorf("LARGE = %+v, want 1", c)
	}
	if c := findCell(g, 2); c == nil || c.Values[0] != 2 {
		t.Errorf("SMALL = %+v, want 2", c)
	}
	if c := findCell(g, 1); c != nil {
		t.Errorf("MEDIUM should have no cell (zero), got %+v", c)
	}
}

type fakeOptionPairs struct{ opts map[string][]CategoryOption }

func (f fakeOptionPairs) DimensionLabels(_ string, src SourceRef) ([]CategoryOption, bool) {
	o, ok := f.opts[src.Key]
	return o, ok
}

func TestEvaluate_DisplaysLabelButGroupsByValue(t *testing.T) {
	// Index stores option values ("open"/"closed"); the axis should show
	// the friendly labels, and a defined-but-absent option ("wip") appears
	// with zero.
	idx := &fakeIndex{
		total: 3,
		raw: []index.StatRawRow{
			{Dims: []string{"open"}}, {Dims: []string{"closed"}}, {Dims: []string{"open"}},
		},
	}
	m := NewManager(idx)
	m.SetSourceOptions(fakeOptionPairs{opts: map[string][]CategoryOption{
		"status": {
			{Value: "open", Label: "Open"},
			{Value: "closed", Label: "Closed"},
			{Value: "wip", Label: "In progress"},
		},
	}})
	g, err := m.Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Open", "Closed", "In progress"}; !reflect.DeepEqual(g.Axes[0].Labels, want) {
		t.Fatalf("labels = %v, want %v", g.Axes[0].Labels, want)
	}
	if c := findCell(g, 0); c == nil || c.Values[0] != 2 {
		t.Errorf("Open (stored 'open') = %+v, want 2", c)
	}
	if c := findCell(g, 1); c == nil || c.Values[0] != 1 {
		t.Errorf("Closed = %+v, want 1", c)
	}
	if c := findCell(g, 2); c != nil {
		t.Errorf("In progress should be zero (no cell), got %+v", c)
	}
}

func TestEvaluate_PresentValueAppendedWhenNotInDefinedSet(t *testing.T) {
	idx := &fakeIndex{
		total: 2,
		raw:   []index.StatRawRow{{Dims: []string{"LARGE"}}, {Dims: []string{"STALE"}}},
	}
	m := NewManager(idx)
	m.SetSourceOptions(fakeOptions{labels: map[string][]string{"tshirt": {"LARGE", "SMALL"}}})
	g, err := m.Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceFacet, Key: "tshirt"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Defined order first (LARGE, SMALL), then stale present value appended.
	if want := []string{"LARGE", "SMALL", "STALE"}; !reflect.DeepEqual(g.Axes[0].Labels, want) {
		t.Errorf("labels = %v, want %v", g.Axes[0].Labels, want)
	}
}

func TestEvaluate_TopN_KeepsBiggestDropsTail(t *testing.T) {
	// Five entities with counts a:1 b:5 c:2 d:4 e:3; top 3 keeps b,d,e
	// (ranked desc), drops a and c.
	idx := &fakeIndex{total: 15}
	for _, r := range []struct {
		label string
		n     int
	}{{"a", 1}, {"b", 5}, {"c", 2}, {"d", 4}, {"e", 3}} {
		for i := 0; i < r.n; i++ {
			idx.raw = append(idx.raw, index.StatRawRow{Dims: []string{r.label}})
		}
	}
	m := NewManager(idx)
	g, err := m.Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "entity"}, Top: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b", "d", "e"}; !reflect.DeepEqual(g.Axes[0].Labels, want) {
		t.Fatalf("labels = %v, want %v (top-3 by count, desc)", g.Axes[0].Labels, want)
	}
	if len(g.Cells) != 3 {
		t.Errorf("cells = %d, want 3 (tail dropped)", len(g.Cells))
	}
	// b is index 0 with count 5.
	if c := findCell(g, 0); c == nil || c.Values[0] != 5 {
		t.Errorf("top cell = %+v, want b=5", c)
	}
	// Total stays the full form count, not the kept sum.
	if g.Total != 15 {
		t.Errorf("total = %d, want 15 (unchanged by top-N)", g.Total)
	}
}

func TestEvaluate_TopN_NoOpWhenFewerCategories(t *testing.T) {
	idx := &fakeIndex{
		total: 2,
		raw:   []index.StatRawRow{{Dims: []string{"x"}}, {Dims: []string{"y"}}},
	}
	g, err := NewManager(idx).Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "f"}, Top: 10}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Axes[0].Labels) != 2 {
		t.Errorf("labels = %v, want both kept (top 10 > 2 categories)", g.Axes[0].Labels)
	}
}

func TestEvaluate_RejectsFacetComparisonFilter(t *testing.T) {
	m := NewManager(&fakeIndex{})
	_, err := m.Evaluate("t", StatConfig{
		Measures: []Measure{{Op: OpCount}},
		Filters:  []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "qzm"}, Op: FilterGt, Value: "5"}},
	})
	if err == nil {
		t.Error("expected rejection: comparison operator on a facet")
	}
}

func TestEvaluate_RejectsTwoTableSourcesViaFilter(t *testing.T) {
	m := NewManager(&fakeIndex{})
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"grp.access": 1, "grp.procedure": 0}})
	_, err := m.Evaluate("t", StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "access"}}},
		Filters:    []Filter{{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "entry"}, Op: FilterEq, Value: "X"}},
	})
	if err == nil {
		t.Error("expected rejection: a table-column dimension plus a table-column filter is two table sources")
	}
}

func TestEvaluate_RejectsEmptyMeasures(t *testing.T) {
	m := NewManager(&fakeIndex{})
	if _, err := m.Evaluate("t", StatConfig{}); err == nil {
		t.Error("expected error for empty measures")
	}
}

func TestManager_EvaluateDSL_ParsesThenEvaluates(t *testing.T) {
	idx := &fakeIndex{
		total: 2,
		raw:   []index.StatRawRow{{Dims: []string{"a"}}, {Dims: []string{"a"}}},
	}
	g, err := NewManager(idx).EvaluateDSL("t", `count() by F["status"]`)
	if err != nil {
		t.Fatal(err)
	}
	if c := findCell(g, 0); c == nil || c.Values[0] != 2 {
		t.Errorf("cell = %+v, want count 2", c)
	}
}

func TestManager_EvaluateDSL_BadDSLErrors(t *testing.T) {
	if _, err := NewManager(&fakeIndex{}).EvaluateDSL("t", "this is not a dsl"); err == nil {
		t.Error("expected a parse error")
	}
}

type fakeSource struct {
	dsl  map[string]string
	list []StatObject
}

func (f fakeSource) StatisticDSL(_, name string) (string, bool, error) {
	d, ok := f.dsl[name]
	return d, ok, nil
}

func (f fakeSource) ListStatistics(_ string) ([]StatObject, error) {
	return f.list, nil
}

func TestService_ListObjects_ReturnsCatalog(t *testing.T) {
	want := []StatObject{
		{Name: "by-status", Label: "By status", DSL: `count() by F["status"]`},
		{Name: "raw", DSL: `count()`},
	}
	svc := NewService(NewManager(&fakeIndex{}), fakeSource{list: want})
	got, err := svc.ListObjects("t")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 || got[0].Name != "by-status" || got[0].Label != "By status" ||
		got[0].DSL != `count() by F["status"]` || got[1].Name != "raw" || got[1].Label != "" {
		t.Fatalf("catalog = %+v", got)
	}
}

func TestService_ListObjects_NoSourceErrors(t *testing.T) {
	svc := NewService(NewManager(&fakeIndex{}), nil)
	if _, err := svc.ListObjects("t"); err == nil {
		t.Fatal("expected error when no source configured")
	}
}

func TestService_EvaluateObject_ResolvesNameThenRuns(t *testing.T) {
	idx := &fakeIndex{
		total: 3,
		raw:   []index.StatRawRow{{Dims: []string{"high"}}, {Dims: []string{"low"}}, {Dims: []string{"high"}}},
	}
	svc := NewService(NewManager(idx), fakeSource{list: []StatObject{{Name: "by-status", DSL: `count() by F["status"]`}}})
	g, err := svc.EvaluateObject("t", "by-status")
	if err != nil {
		t.Fatal(err)
	}
	if g.Total != 3 || len(g.Axes) != 1 {
		t.Fatalf("grid = %+v", g)
	}
	if c := findCell(g, 0); c == nil || c.Values[0] != 2 {
		t.Errorf("high cell = %+v, want count 2", c)
	}
}

func TestService_EvaluateObject_UnknownNameErrors(t *testing.T) {
	svc := NewService(NewManager(&fakeIndex{}), fakeSource{list: []StatObject{}})
	if _, err := svc.EvaluateObject("t", "ghost"); err == nil {
		t.Error("expected error for unknown statistic name")
	}
}

func TestService_EvaluateObject_NilSourceErrors(t *testing.T) {
	svc := NewService(NewManager(&fakeIndex{}), nil)
	if _, err := svc.EvaluateObject("t", "x"); err == nil {
		t.Error("expected error when no source configured")
	}
}
