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

func (f *fakeIndex) AggregateRaw(string, []index.AggDim, []index.AggNum) ([]index.StatRawRow, error) {
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

func TestDistribution_ShapesBucketsAndTotal(t *testing.T) {
	m := NewManager(&fakeIndex{
		total:   5,
		valDist: []index.Bucket{{Label: "high", Count: 2}, {Label: "low", Count: 1}},
	})
	res, err := m.Distribution("t", "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindDistribution {
		t.Errorf("kind = %q", res.Kind)
	}
	if res.Total != 5 {
		t.Errorf("total = %d, want 5", res.Total)
	}
	if len(res.Categories) != 2 || res.Categories[0] != "high" {
		t.Errorf("categories = %v", res.Categories)
	}
	if len(res.Series) != 1 || res.Series[0].Name != "count" {
		t.Fatalf("series = %+v", res.Series)
	}
	if res.Series[0].Values[0] != 2 || res.Series[0].Values[1] != 1 {
		t.Errorf("values = %v, want [2 1]", res.Series[0].Values)
	}
}

func TestNumericStats_FlattensSummary(t *testing.T) {
	m := NewManager(&fakeIndex{total: 3, nums: []float64{10, 20, 30}})
	res, err := m.NumericStats("t", "amount", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindScalarStats {
		t.Errorf("kind = %q", res.Kind)
	}
	if res.Scalars["sum"] != 60 || res.Scalars["avg"] != 20 || res.Scalars["count"] != 3 {
		t.Errorf("scalars = %v", res.Scalars)
	}
}

func TestNumericStats_EmptyHasZeroCount(t *testing.T) {
	m := NewManager(&fakeIndex{total: 3, nums: nil})
	res, err := m.NumericStats("t", "amount", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scalars["count"] != 0 {
		t.Errorf("empty count = %v, want 0", res.Scalars["count"])
	}
}

func TestTimeSeries_KindAndOrder(t *testing.T) {
	m := NewManager(&fakeIndex{
		total: 3,
		dates: []index.Bucket{{Label: "2026-01", Count: 2}, {Label: "2026-02", Count: 1}},
	})
	res, err := m.TimeSeries("t", "due", nil, "month")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries {
		t.Errorf("kind = %q, want timeseries", res.Kind)
	}
	if res.Categories[0] != "2026-01" || res.Categories[1] != "2026-02" {
		t.Errorf("categories = %v", res.Categories)
	}
}

func TestCrossTab_MatrixZeroFilled(t *testing.T) {
	// A in {p,q}, B in {x,y}; only (p,x), (p,y), (q,x) present. (q,y)
	// must zero-fill.
	m := NewManager(&fakeIndex{
		total: 4,
		cross: []index.CrossCell{
			{A: "p", B: "x", Count: 1},
			{A: "p", B: "y", Count: 2},
			{A: "q", B: "x", Count: 3},
		},
	})
	res, err := m.CrossTab("t", "ka", "kb")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindCrosstab {
		t.Errorf("kind = %q", res.Kind)
	}
	if len(res.Categories) != 2 || res.Categories[0] != "p" || res.Categories[1] != "q" {
		t.Fatalf("categories = %v, want [p q]", res.Categories)
	}
	// series are per-B: x and y.
	byName := map[string][]float64{}
	for _, s := range res.Series {
		byName[s.Name] = s.Values
	}
	if got := byName["x"]; len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("series x = %v, want [1 3]", got)
	}
	if got := byName["y"]; len(got) != 2 || got[0] != 2 || got[1] != 0 {
		t.Errorf("series y = %v, want [2 0] (q,y zero-filled)", got)
	}
}
