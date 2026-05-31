package statengine

import (
	"reflect"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// columnKeyIn: a non-string value type is not a usable key.
func TestColumnKeyIn_NonStringValue(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{map[string]any{"value": 42}}},
	}}
	got, ok := columnKeyIn(tpl, "rows", 0)
	if ok || got != "" {
		t.Fatalf("columnKeyIn = %q,%v want empty,false", got, ok)
	}
}

// columnKeyIn: a map option with no value entry is not a usable key.
func TestColumnKeyIn_MissingValueKey(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{map[string]any{"label": "a"}}},
	}}
	_, ok := columnKeyIn(tpl, "rows", 0)
	if ok {
		t.Fatal("want ok=false when value key absent")
	}
}

// columnKeyIn: column zero on a field that has no options is out of range.
func TestColumnKeyIn_EmptyOptions(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: nil},
	}}
	_, ok := columnKeyIn(tpl, "rows", 0)
	if ok {
		t.Fatal("want ok=false for column 0 on empty options")
	}
}

// columnKeyIn: the first matching field by key wins even with later duplicates.
func TestColumnKeyIn_FirstFieldKeyMatchWins(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{map[string]any{"value": "first"}}},
		{Key: "rows", Options: []any{map[string]any{"value": "second"}}},
	}}
	got, ok := columnKeyIn(tpl, "rows", 0)
	if !ok || got != "first" {
		t.Fatalf("columnKeyIn = %q,%v want first,true", got, ok)
	}
}

// columnKeyIn on a template with no fields at all returns ok=false.
func TestColumnKeyIn_NoFields(t *testing.T) {
	tpl := &template.Template{}
	_, ok := columnKeyIn(tpl, "rows", 0)
	if ok {
		t.Fatal("want ok=false when template has no fields")
	}
}

// ValueDistribution: a scalar field (nil col) must not consult the namer.
func TestValueDistribution_ScalarSkipsNamer(t *testing.T) {
	fr := &fakeReducer{dist: []datacore.Bucket{{Value: "z", Count: 9}}}
	a := New(fr, fakeNamer{keys: map[int]string{0: "should-not-be-read"}})
	out, err := a.ValueDistribution("t.yaml", "status", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []index.Bucket{{Label: "z", Count: 9}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("buckets = %#v, want %#v", out, want)
	}
	if fr.distTpl != "t.yaml" || fr.distFol != "" || fr.distFld != "status" {
		t.Fatalf("dist args = %q,%q,%q want t.yaml,empty,status", fr.distTpl, fr.distFol, fr.distFld)
	}
}

// ValueDistribution: an empty reducer slice maps to an empty non-nil slice.
func TestValueDistribution_EmptyBucketsNonNil(t *testing.T) {
	a := New(&fakeReducer{dist: []datacore.Bucket{}}, fakeNamer{})
	out, err := a.ValueDistribution("t.yaml", "f", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("out = %#v, want empty non-nil", out)
	}
}

// NumericValues: nil reducer Values round-trips as nil.
func TestNumericValues_NilValues(t *testing.T) {
	a := New(&fakeReducer{agg: datacore.Aggregate{N: 0, Values: nil}}, fakeNamer{})
	out, err := a.NumericValues("t.yaml", "score", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != nil {
		t.Fatalf("out = %#v, want nil", out)
	}
}

// DateSeries: an empty series maps to an empty non-nil bucket slice.
func TestDateSeries_EmptySeriesNonNil(t *testing.T) {
	a := New(&fakeReducer{series: datacore.Series{}}, fakeNamer{})
	out, err := a.DateSeries("t.yaml", "created", nil, "day")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("out = %#v, want empty non-nil", out)
	}
}

// FacetCross: the cell slice length and order mirror the reducer cells exactly.
func TestFacetCross_PreservesOrderAndLength(t *testing.T) {
	fr := &fakeReducer{cross: datacore.CrossTab{Cells: []datacore.CrossCell{
		{Row: "r1", Col: "c1", Count: 5},
		{Row: "r1", Col: "c2", Count: 0},
		{Row: "r2", Col: "c1", Count: 7},
	}}}
	a := New(fr, fakeNamer{})
	out, err := a.FacetCross("t.yaml", "ka", "kb")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []index.CrossCell{
		{A: "r1", B: "c1", Count: 5},
		{A: "r1", B: "c2", Count: 0},
		{A: "r2", B: "c1", Count: 7},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("cells = %#v, want %#v", out, want)
	}
}

// AggregateRaw: a facet-kind filter with Col set still errors; Col is checked
// before Kind, and the message reports the raw (non-namespaced) key.
func TestAggregateRaw_FacetFilterWithColErrors(t *testing.T) {
	a := New(&fakeReducer{}, fakeNamer{})
	filters := []index.AggFilter{{Kind: "facet", Key: "prio", Col: intp(4), Op: "eq", Value: "hi"}}
	_, err := a.AggregateRaw("t.yaml", nil, nil, filters)
	if err == nil {
		t.Fatal("want error for facet filter with column")
	}
	if got := err.Error(); got != `statengine: table-column filters not supported yet (table "prio" col 4)` {
		t.Fatalf("err = %q", got)
	}
}

// AggregateRaw: multiple rows preserve order, form names, and per-row NullFloat64
// validity flags through the translation.
func TestAggregateRaw_MultipleRowsNumValidity(t *testing.T) {
	fr := &fakeReducer{raw: []datacore.GridRow{
		{Form: "a.md", Dims: []string{"g1"}, Nums: []datacore.NumCell{{Value: 1, OK: true}}},
		{Form: "b.md", Dims: []string{"g2"}, Nums: []datacore.NumCell{{Value: 9, OK: false}}},
	}}
	a := New(fr, fakeNamer{})
	out, err := a.AggregateRaw("t.yaml", nil, []index.AggNum{{Key: "score"}}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("rows = %d, want 2", len(out))
	}
	if out[0].Form != "a.md" || !out[0].Nums[0].Valid || out[0].Nums[0].Float64 != 1 {
		t.Fatalf("row0 = %#v", out[0])
	}
	if out[1].Form != "b.md" || out[1].Nums[0].Valid {
		t.Fatalf("row1 num should be invalid, got %#v", out[1])
	}
	if out[1].Nums[0].Float64 != 9 {
		t.Fatalf("row1 float carried even when invalid = %v, want 9", out[1].Nums[0].Float64)
	}
}

// AggregateRaw: a scalar num (nil Col) maps to an empty table with the key as
// the field, never consulting the namer.
func TestAggregateRaw_ScalarNumPassthrough(t *testing.T) {
	fr := &fakeReducer{raw: []datacore.GridRow{}}
	a := New(fr, fakeNamer{keys: map[int]string{0: "unused"}})
	_, err := a.AggregateRaw("t.yaml", nil, []index.AggNum{{Key: "score", Col: nil}}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []datacore.GridNum{{Field: "score", Table: ""}}
	if !reflect.DeepEqual(fr.rawNums, want) {
		t.Fatalf("gridNums = %#v, want %#v", fr.rawNums, want)
	}
}

// AggregateRaw: dim translation aborts on the first bad column; later dims and
// the reducer are never reached.
func TestAggregateRaw_DimErrorShortCircuits(t *testing.T) {
	fr := &fakeReducer{}
	a := New(fr, fakeNamer{keys: map[int]string{0: "ok"}})
	dims := []index.AggDim{
		{Kind: "field", Key: "rows", Col: intp(0)},
		{Kind: "field", Key: "rows", Col: intp(99)},
	}
	_, err := a.AggregateRaw("t.yaml", dims, nil, nil)
	if err == nil {
		t.Fatal("want error for bad second dim")
	}
	if got := err.Error(); got != `statengine: no column 99 in table "rows" of "t.yaml"` {
		t.Fatalf("err = %q", got)
	}
	if fr.rawDims != nil {
		t.Fatalf("reducer should not be called, rawDims = %#v", fr.rawDims)
	}
}

// facetField is a pure namespacing prefix even for an empty key.
func TestFacetField_EmptyKey(t *testing.T) {
	if got := facetField(""); got != "facet:" {
		t.Fatalf("facetField = %q, want facet:", got)
	}
}

// roReducer is a stateless reducer: it records nothing, so concurrent use is
// safe and exercises the adapter (not the fake) under the race detector.
type roReducer struct{}

func (roReducer) Count(string, string) (int, error) { return 4, nil }
func (roReducer) Distribution(string, string, string) ([]datacore.Bucket, error) {
	return []datacore.Bucket{{Value: "a", Count: 1}}, nil
}
func (roReducer) Aggregate(string, string, string) (datacore.Aggregate, error) {
	return datacore.Aggregate{Values: []float64{2}}, nil
}
func (roReducer) Cross(string, string, string, string) (datacore.CrossTab, error) {
	return datacore.CrossTab{Cells: []datacore.CrossCell{{Row: "r", Col: "c", Count: 1}}}, nil
}
func (roReducer) DateSeries(string, string, string, string) (datacore.Series, error) {
	return datacore.Series{Buckets: []datacore.Bucket{{Value: "p", Count: 1}}}, nil
}
func (roReducer) AggregateRaw(string, []datacore.GridDim, []datacore.GridNum, []datacore.GridFilter) ([]datacore.GridRow, error) {
	return nil, nil
}

// The adapter is stateless and safe for concurrent reads across methods.
func TestDatacoreIndex_ConcurrentReadsRace(t *testing.T) {
	a := New(roReducer{}, fakeNamer{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n, err := a.TotalForms("t.yaml"); err != nil || n != 4 {
				t.Errorf("TotalForms = %d,%v", n, err)
			}
			if _, err := a.ValueDistribution("t.yaml", "f", nil); err != nil {
				t.Errorf("ValueDistribution err: %v", err)
			}
			if _, err := a.FacetCross("t.yaml", "x", "y"); err != nil {
				t.Errorf("FacetCross err: %v", err)
			}
		}()
	}
	wg.Wait()
}

// New wires the given reducer so TotalForms reaches it and returns its count.
func TestNew_WiresReducer(t *testing.T) {
	fr := &fakeReducer{countN: 11}
	a := New(fr, fakeNamer{})
	if a == nil {
		t.Fatal("New returned nil")
	}
	n, err := a.TotalForms("t.yaml")
	if err != nil || n != 11 {
		t.Fatalf("TotalForms = %d,%v want 11,nil", n, err)
	}
}
