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

func TestEvaluate_RejectsTableColumnSources(t *testing.T) {
	m := NewManager(&fakeIndex{})
	cases := []StatConfig{
		{
			Measures:   []Measure{{Op: OpCount}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "items", Column: "qty"}}},
		},
		{
			Measures: []Measure{{Op: OpSum, Source: &SourceRef{Kind: SourceField, Key: "items", Column: "qty"}}},
		},
	}
	for i, cfg := range cases {
		if _, err := m.Evaluate("t", cfg); err == nil {
			t.Errorf("[%d] expected table-column rejection, got nil", i)
		}
	}
}

func TestEvaluate_RejectsEmptyMeasures(t *testing.T) {
	m := NewManager(&fakeIndex{})
	if _, err := m.Evaluate("t", StatConfig{}); err == nil {
		t.Error("expected error for empty measures")
	}
}
