package stat

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// fakeIndex is a hand-fed Index for shaping tests - no SQLite. The
// index module's own tests cover that the real queries return these
// shapes; here we only assert the chart-neutral transformation.
type fakeIndex struct {
	total   int
	valDist []index.Bucket
	nums    []float64
	facet   []index.Bucket
	cross   []index.CrossCell
	dates   []index.Bucket
	raw     []index.StatRawRow
}

func (f *fakeIndex) AggregateRaw(string, []index.AggDim, []index.AggNum, []index.AggFilter) ([]index.StatRawRow, error) {
	return f.raw, nil
}

func (f *fakeIndex) TotalForms(string) (int, error) { return f.total, nil }
func (f *fakeIndex) ValueDistribution(string, string, *int) ([]index.Bucket, error) {
	return f.valDist, nil
}
func (f *fakeIndex) NumericValues(string, string, *int) ([]float64, error)   { return f.nums, nil }
func (f *fakeIndex) FacetDistribution(string, string) ([]index.Bucket, error) { return f.facet, nil }
func (f *fakeIndex) FacetCross(string, string, string) ([]index.CrossCell, error) {
	return f.cross, nil
}
func (f *fakeIndex) DateSeries(string, string, *int, string) ([]index.Bucket, error) {
	return f.dates, nil
}

// cell1 reads the rank-1 cell value at axis-0 index i (0 if absent).
func cell1(g *Grid, i int) float64 {
	for _, c := range g.Cells {
		if len(c.Coords) == 1 && c.Coords[0] == i {
			return c.Values[0]
		}
	}
	return 0
}

// cell2 reads the rank-2 cell value at (a, b) (0 if absent).
func cell2(g *Grid, a, b int) float64 {
	for _, c := range g.Cells {
		if len(c.Coords) == 2 && c.Coords[0] == a && c.Coords[1] == b {
			return c.Values[0]
		}
	}
	return 0
}

// measure reads the rank-0 cell's value for a named measure (-1 if absent).
func measure(g *Grid, name string) float64 {
	for i, m := range g.Measures {
		if m == name {
			return g.Cells[0].Values[i]
		}
	}
	return -1
}

func TestDistribution_ShapesGridAndTotal(t *testing.T) {
	m := NewManager(&fakeIndex{
		total:   5,
		valDist: []index.Bucket{{Label: "high", Count: 2}, {Label: "low", Count: 1}},
	})
	g, err := m.Distribution("t", "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Total != 5 {
		t.Errorf("total = %d, want 5", g.Total)
	}
	if len(g.Axes) != 1 || g.Axes[0].Source != "status" {
		t.Fatalf("axes = %+v, want one axis sourced 'status'", g.Axes)
	}
	if len(g.Axes[0].Labels) != 2 || g.Axes[0].Labels[0] != "high" {
		t.Errorf("labels = %v", g.Axes[0].Labels)
	}
	if len(g.Measures) != 1 || g.Measures[0] != "count" {
		t.Errorf("measures = %v", g.Measures)
	}
	if cell1(g, 0) != 2 || cell1(g, 1) != 1 {
		t.Errorf("values = [%v %v], want [2 1]", cell1(g, 0), cell1(g, 1))
	}
}

func TestFacetDistribution_AxisSourceIsFacetKey(t *testing.T) {
	// The axis source must be the facet key so a renderer can color
	// categories with the facet's authored option colors.
	m := NewManager(&fakeIndex{
		total: 3,
		facet: []index.Bucket{{Label: "AANWEZIG", Count: 2}},
	})
	g, err := m.FacetDistribution("t", "fcdm")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Axes) != 1 || g.Axes[0].Source != "fcdm" {
		t.Fatalf("axes = %+v, want source 'fcdm'", g.Axes)
	}
}

func TestNumericStats_Rank0Measures(t *testing.T) {
	m := NewManager(&fakeIndex{total: 3, nums: []float64{10, 20, 30}})
	g, err := m.NumericStats("t", "amount", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Axes) != 0 {
		t.Errorf("rank-0 expected, got axes %+v", g.Axes)
	}
	if measure(g, "sum") != 60 || measure(g, "avg") != 20 || measure(g, "count") != 3 {
		t.Errorf("measures wrong: sum=%v avg=%v count=%v", measure(g, "sum"), measure(g, "avg"), measure(g, "count"))
	}
}

func TestNumericStats_EmptyHasZeroCount(t *testing.T) {
	m := NewManager(&fakeIndex{total: 3, nums: nil})
	g, err := m.NumericStats("t", "amount", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if measure(g, "count") != 0 {
		t.Errorf("empty count = %v, want 0", measure(g, "count"))
	}
}

func TestTimeSeries_Rank1Order(t *testing.T) {
	m := NewManager(&fakeIndex{
		total: 3,
		dates: []index.Bucket{{Label: "2026-01", Count: 2}, {Label: "2026-02", Count: 1}},
	})
	g, err := m.TimeSeries("t", "due", nil, "month")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Axes) != 1 || g.Axes[0].Labels[0] != "2026-01" || g.Axes[0].Labels[1] != "2026-02" {
		t.Errorf("labels = %v, want [2026-01 2026-02]", g.Axes[0].Labels)
	}
}

func TestCrossTab_Rank2ZeroFilled(t *testing.T) {
	// A in {p,q}, B in {x,y}; only (p,x), (p,y), (q,x) present. (q,y)
	// has no cell (reads 0).
	m := NewManager(&fakeIndex{
		total: 4,
		cross: []index.CrossCell{
			{A: "p", B: "x", Count: 1},
			{A: "p", B: "y", Count: 2},
			{A: "q", B: "x", Count: 3},
		},
	})
	g, err := m.CrossTab("t", "ka", "kb")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Axes) != 2 || g.Axes[0].Source != "ka" || g.Axes[1].Source != "kb" {
		t.Fatalf("axes = %+v", g.Axes)
	}
	if len(g.Axes[0].Labels) != 2 || g.Axes[0].Labels[0] != "p" || g.Axes[0].Labels[1] != "q" {
		t.Fatalf("axis0 labels = %v, want [p q]", g.Axes[0].Labels)
	}
	// sorted: p=0,q=1 / x=0,y=1.
	if cell2(g, 0, 0) != 1 || cell2(g, 0, 1) != 2 || cell2(g, 1, 0) != 3 {
		t.Errorf("cells wrong: (p,x)=%v (p,y)=%v (q,x)=%v", cell2(g, 0, 0), cell2(g, 0, 1), cell2(g, 1, 0))
	}
	if cell2(g, 1, 1) != 0 {
		t.Errorf("(q,y) should be 0, got %v", cell2(g, 1, 1))
	}
}
