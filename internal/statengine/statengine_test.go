package statengine

import (
	"errors"
	"reflect"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// fakeReducer records the last call args per method and returns canned values.
type fakeReducer struct {
	countN   int
	countErr error
	countTpl string
	countFol string

	dist    []datacore.Bucket
	distErr error
	distTpl string
	distFol string
	distFld string

	agg    datacore.Aggregate
	aggErr error
	aggTpl string
	aggFol string
	aggFld string

	cross    datacore.CrossTab
	crossErr error
	crossRow string
	crossCol string

	series    datacore.Series
	seriesErr error
	seriesPer string

	raw     []datacore.GridRow
	rawErr  error
	rawDims []datacore.GridDim
	rawNums []datacore.GridNum
	rawFlts []datacore.GridFilter
}

func (f *fakeReducer) Count(tpl, follow string) (int, error) {
	f.countTpl, f.countFol = tpl, follow
	return f.countN, f.countErr
}

func (f *fakeReducer) Distribution(tpl, follow, field string) ([]datacore.Bucket, error) {
	f.distTpl, f.distFol, f.distFld = tpl, follow, field
	return f.dist, f.distErr
}

func (f *fakeReducer) Aggregate(tpl, follow, field string) (datacore.Aggregate, error) {
	f.aggTpl, f.aggFol, f.aggFld = tpl, follow, field
	return f.agg, f.aggErr
}

func (f *fakeReducer) Cross(tpl, follow, rowField, colField string) (datacore.CrossTab, error) {
	f.crossRow, f.crossCol = rowField, colField
	return f.cross, f.crossErr
}

func (f *fakeReducer) DateSeries(tpl, follow, field, period string) (datacore.Series, error) {
	f.seriesPer = period
	return f.series, f.seriesErr
}

func (f *fakeReducer) AggregateRaw(tpl string, dims []datacore.GridDim, nums []datacore.GridNum, filters []datacore.GridFilter) ([]datacore.GridRow, error) {
	f.rawDims, f.rawNums, f.rawFlts = dims, nums, filters
	return f.raw, f.rawErr
}

// fakeNamer maps (tplFile, fieldKey, col) to a column key via a lookup map. A
// missing entry returns ok=false.
type fakeNamer struct {
	keys map[int]string
}

func (n fakeNamer) ColumnKey(tplFile, fieldKey string, col int) (string, bool) {
	if n.keys == nil {
		return "", false
	}
	k, ok := n.keys[col]
	return k, ok
}

func intp(i int) *int { return &i }

func TestTotalForms_PassesThroughCountAndFollowEmpty(t *testing.T) {
	fr := &fakeReducer{countN: 7}
	a := New(fr, fakeNamer{})
	n, err := a.TotalForms("t.yaml")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 7 {
		t.Fatalf("count = %d, want 7", n)
	}
	if fr.countTpl != "t.yaml" || fr.countFol != "" {
		t.Fatalf("count args = %q,%q want t.yaml,empty", fr.countTpl, fr.countFol)
	}
}

func TestTotalForms_PropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	a := New(&fakeReducer{countErr: sentinel}, fakeNamer{})
	_, err := a.TotalForms("t.yaml")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestValueDistribution_ScalarFollowsNothing(t *testing.T) {
	fr := &fakeReducer{dist: []datacore.Bucket{{Value: "a", Count: 2}, {Value: "b", Count: 5}}}
	a := New(fr, fakeNamer{})
	out, err := a.ValueDistribution("t.yaml", "status", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []index.Bucket{{Label: "a", Count: 2}, {Label: "b", Count: 5}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("buckets = %#v, want %#v", out, want)
	}
	if fr.distFol != "" || fr.distFld != "status" {
		t.Fatalf("follow,field = %q,%q want empty,status", fr.distFol, fr.distFld)
	}
}

func TestValueDistribution_TableColumnFollowsTable(t *testing.T) {
	fr := &fakeReducer{dist: []datacore.Bucket{{Value: "x", Count: 1}}}
	a := New(fr, fakeNamer{keys: map[int]string{2: "amount"}})
	out, err := a.ValueDistribution("t.yaml", "rows", intp(2))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 1 || out[0].Label != "x" || out[0].Count != 1 {
		t.Fatalf("buckets = %#v", out)
	}
	if fr.distFol != "rows" || fr.distFld != "amount" {
		t.Fatalf("follow,field = %q,%q want rows,amount", fr.distFol, fr.distFld)
	}
}

func TestValueDistribution_UnknownColumnYieldsNilNoError(t *testing.T) {
	fr := &fakeReducer{dist: []datacore.Bucket{{Value: "x", Count: 1}}}
	a := New(fr, fakeNamer{keys: map[int]string{0: "k"}})
	out, err := a.ValueDistribution("t.yaml", "rows", intp(9))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != nil {
		t.Fatalf("out = %#v, want nil", out)
	}
	if fr.distTpl != "" {
		t.Fatalf("reducer should not be called, distTpl = %q", fr.distTpl)
	}
}

func TestValueDistribution_PropagatesError(t *testing.T) {
	sentinel := errors.New("dist fail")
	a := New(&fakeReducer{distErr: sentinel}, fakeNamer{})
	_, err := a.ValueDistribution("t.yaml", "f", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestNumericValues_ReturnsAggregateValues(t *testing.T) {
	fr := &fakeReducer{agg: datacore.Aggregate{N: 3, Values: []float64{1.5, 2, 3}}}
	a := New(fr, fakeNamer{})
	out, err := a.NumericValues("t.yaml", "score", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !reflect.DeepEqual(out, []float64{1.5, 2, 3}) {
		t.Fatalf("values = %#v", out)
	}
	if fr.aggFol != "" || fr.aggFld != "score" {
		t.Fatalf("follow,field = %q,%q want empty,score", fr.aggFol, fr.aggFld)
	}
}

func TestNumericValues_UnknownColumnYieldsNilNoError(t *testing.T) {
	fr := &fakeReducer{agg: datacore.Aggregate{Values: []float64{9}}}
	a := New(fr, fakeNamer{keys: map[int]string{0: "k"}})
	out, err := a.NumericValues("t.yaml", "rows", intp(5))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != nil {
		t.Fatalf("out = %#v, want nil", out)
	}
	if fr.aggTpl != "" {
		t.Fatalf("reducer should not be called, aggTpl = %q", fr.aggTpl)
	}
}

func TestNumericValues_PropagatesError(t *testing.T) {
	sentinel := errors.New("agg fail")
	a := New(&fakeReducer{aggErr: sentinel}, fakeNamer{})
	_, err := a.NumericValues("t.yaml", "f", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestFacetDistribution_NamespacesFacetField(t *testing.T) {
	fr := &fakeReducer{dist: []datacore.Bucket{{Value: "hi", Count: 4}}}
	a := New(fr, fakeNamer{})
	out, err := a.FacetDistribution("t.yaml", "priority")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 1 || out[0].Label != "hi" || out[0].Count != 4 {
		t.Fatalf("buckets = %#v", out)
	}
	if fr.distFld != "facet:priority" || fr.distFol != "" {
		t.Fatalf("field,follow = %q,%q want facet:priority,empty", fr.distFld, fr.distFol)
	}
}

func TestFacetDistribution_PropagatesError(t *testing.T) {
	sentinel := errors.New("facet fail")
	a := New(&fakeReducer{distErr: sentinel}, fakeNamer{})
	_, err := a.FacetDistribution("t.yaml", "p")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestFacetCross_MapsCellsAndNamespacesBothKeys(t *testing.T) {
	fr := &fakeReducer{cross: datacore.CrossTab{
		Cells: []datacore.CrossCell{
			{Row: "a", Col: "x", Count: 1},
			{Row: "b", Col: "y", Count: 3},
		},
	}}
	a := New(fr, fakeNamer{})
	out, err := a.FacetCross("t.yaml", "k1", "k2")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []index.CrossCell{{A: "a", B: "x", Count: 1}, {A: "b", B: "y", Count: 3}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("cells = %#v, want %#v", out, want)
	}
	if fr.crossRow != "facet:k1" || fr.crossCol != "facet:k2" {
		t.Fatalf("row,col = %q,%q want facet:k1,facet:k2", fr.crossRow, fr.crossCol)
	}
}

func TestFacetCross_EmptyCellsYieldsEmptySlice(t *testing.T) {
	a := New(&fakeReducer{cross: datacore.CrossTab{}}, fakeNamer{})
	out, err := a.FacetCross("t.yaml", "k1", "k2")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("out = %#v, want empty non-nil", out)
	}
}

func TestFacetCross_PropagatesError(t *testing.T) {
	sentinel := errors.New("cross fail")
	a := New(&fakeReducer{crossErr: sentinel}, fakeNamer{})
	_, err := a.FacetCross("t.yaml", "k1", "k2")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestDateSeries_MapsSeriesBucketsAndPassesPeriod(t *testing.T) {
	fr := &fakeReducer{series: datacore.Series{Buckets: []datacore.Bucket{
		{Value: "2026-01", Count: 2},
		{Value: "2026-02", Count: 5},
	}}}
	a := New(fr, fakeNamer{})
	out, err := a.DateSeries("t.yaml", "created", nil, "month")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []index.Bucket{{Label: "2026-01", Count: 2}, {Label: "2026-02", Count: 5}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("buckets = %#v, want %#v", out, want)
	}
	if fr.seriesPer != "month" {
		t.Fatalf("period = %q, want month", fr.seriesPer)
	}
}

func TestDateSeries_UnknownColumnYieldsNilNoError(t *testing.T) {
	fr := &fakeReducer{series: datacore.Series{Buckets: []datacore.Bucket{{Value: "v", Count: 1}}}}
	a := New(fr, fakeNamer{keys: map[int]string{0: "k"}})
	out, err := a.DateSeries("t.yaml", "rows", intp(7), "day")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != nil {
		t.Fatalf("out = %#v, want nil", out)
	}
	if fr.seriesPer != "" {
		t.Fatalf("reducer should not be called, period = %q", fr.seriesPer)
	}
}

func TestDateSeries_PropagatesError(t *testing.T) {
	sentinel := errors.New("series fail")
	a := New(&fakeReducer{seriesErr: sentinel}, fakeNamer{})
	_, err := a.DateSeries("t.yaml", "f", nil, "year")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestAggregateRaw_TranslatesDimsNumsFiltersAndRows(t *testing.T) {
	fr := &fakeReducer{raw: []datacore.GridRow{
		{Form: "f1.md", Dims: []string{"a"}, Nums: []datacore.NumCell{{Value: 2.5, OK: true}, {Value: 0, OK: false}}},
	}}
	a := New(fr, fakeNamer{keys: map[int]string{1: "col1"}})
	dims := []index.AggDim{
		{Kind: "field", Key: "status", DateWidth: 7},
		{Kind: "facet", Key: "prio"},
		{Kind: "field", Key: "rows", Col: intp(1)},
	}
	nums := []index.AggNum{{Key: "score"}}
	filters := []index.AggFilter{
		{Kind: "field", Key: "state", Op: "eq", Value: "open"},
		{Kind: "facet", Key: "prio", Op: "ne", Value: "low"},
	}
	out, err := a.AggregateRaw("t.yaml", dims, nums, filters)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	wantDims := []datacore.GridDim{
		{Field: "status", Table: "", DateWidth: 7},
		{Field: "facet:prio", Table: "", DateWidth: 0},
		{Field: "col1", Table: "rows", DateWidth: 0},
	}
	if !reflect.DeepEqual(fr.rawDims, wantDims) {
		t.Fatalf("gridDims = %#v, want %#v", fr.rawDims, wantDims)
	}
	wantNums := []datacore.GridNum{{Field: "score", Table: ""}}
	if !reflect.DeepEqual(fr.rawNums, wantNums) {
		t.Fatalf("gridNums = %#v, want %#v", fr.rawNums, wantNums)
	}
	wantFlts := []datacore.GridFilter{
		{Field: "state", Op: "eq", Value: "open"},
		{Field: "facet:prio", Op: "ne", Value: "low"},
	}
	if !reflect.DeepEqual(fr.rawFlts, wantFlts) {
		t.Fatalf("gridFilters = %#v, want %#v", fr.rawFlts, wantFlts)
	}

	if len(out) != 1 {
		t.Fatalf("rows = %d, want 1", len(out))
	}
	r := out[0]
	if r.Form != "f1.md" || !reflect.DeepEqual(r.Dims, []string{"a"}) {
		t.Fatalf("row form/dims = %q/%#v", r.Form, r.Dims)
	}
	if len(r.Nums) != 2 {
		t.Fatalf("nums len = %d, want 2", len(r.Nums))
	}
	if !r.Nums[0].Valid || r.Nums[0].Float64 != 2.5 {
		t.Fatalf("num0 = %#v, want {2.5 valid}", r.Nums[0])
	}
	if r.Nums[1].Valid {
		t.Fatalf("num1 should be invalid, got %#v", r.Nums[1])
	}
}

func TestAggregateRaw_UnknownDimColumnErrors(t *testing.T) {
	a := New(&fakeReducer{}, fakeNamer{keys: map[int]string{0: "k"}})
	dims := []index.AggDim{{Kind: "field", Key: "rows", Col: intp(3)}}
	_, err := a.AggregateRaw("t.yaml", dims, nil, nil)
	if err == nil {
		t.Fatal("want error for unknown dim column")
	}
	if got := err.Error(); got != `statengine: no column 3 in table "rows" of "t.yaml"` {
		t.Fatalf("err = %q", got)
	}
}

func TestAggregateRaw_UnknownNumColumnErrors(t *testing.T) {
	a := New(&fakeReducer{}, fakeNamer{keys: map[int]string{0: "k"}})
	nums := []index.AggNum{{Key: "rows", Col: intp(8)}}
	_, err := a.AggregateRaw("t.yaml", nil, nums, nil)
	if err == nil {
		t.Fatal("want error for unknown num column")
	}
	if got := err.Error(); got != `statengine: no column 8 in table "rows" of "t.yaml"` {
		t.Fatalf("err = %q", got)
	}
}

func TestAggregateRaw_TableColumnFilterErrors(t *testing.T) {
	a := New(&fakeReducer{}, fakeNamer{})
	filters := []index.AggFilter{{Kind: "field", Key: "rows", Col: intp(2), Op: "eq", Value: "v"}}
	_, err := a.AggregateRaw("t.yaml", nil, nil, filters)
	if err == nil {
		t.Fatal("want error for table-column filter")
	}
	if got := err.Error(); got != `statengine: table-column filters not supported yet (table "rows" col 2)` {
		t.Fatalf("err = %q", got)
	}
}

func TestAggregateRaw_PropagatesReducerError(t *testing.T) {
	sentinel := errors.New("raw fail")
	a := New(&fakeReducer{rawErr: sentinel}, fakeNamer{})
	_, err := a.AggregateRaw("t.yaml", nil, nil, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestAggregateRaw_EmptyInputsYieldEmptyRows(t *testing.T) {
	fr := &fakeReducer{raw: []datacore.GridRow{}}
	a := New(fr, fakeNamer{})
	out, err := a.AggregateRaw("t.yaml", nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("out = %#v, want empty non-nil", out)
	}
	if fr.rawDims == nil || len(fr.rawDims) != 0 {
		t.Fatalf("gridDims = %#v, want empty non-nil", fr.rawDims)
	}
	if fr.rawFlts == nil || len(fr.rawFlts) != 0 {
		t.Fatalf("gridFilters = %#v, want empty non-nil", fr.rawFlts)
	}
}

func TestColumnKeyIn_ResolvesValueOption(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{
			map[string]any{"value": "alpha"},
			map[string]any{"value": "beta"},
		}},
	}}
	got, ok := columnKeyIn(tpl, "rows", 1)
	if !ok || got != "beta" {
		t.Fatalf("columnKeyIn = %q,%v want beta,true", got, ok)
	}
}

func TestColumnKeyIn_NegativeColumnOutOfRange(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{map[string]any{"value": "a"}}},
	}}
	_, ok := columnKeyIn(tpl, "rows", -1)
	if ok {
		t.Fatal("want ok=false for negative column")
	}
}

func TestColumnKeyIn_ColumnBeyondOptions(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{map[string]any{"value": "a"}}},
	}}
	_, ok := columnKeyIn(tpl, "rows", 1)
	if ok {
		t.Fatal("want ok=false for column past options length")
	}
}

func TestColumnKeyIn_UnknownFieldKey(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{map[string]any{"value": "a"}}},
	}}
	_, ok := columnKeyIn(tpl, "missing", 0)
	if ok {
		t.Fatal("want ok=false for unknown field key")
	}
}

func TestColumnKeyIn_EmptyValueOption(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{map[string]any{"value": ""}}},
	}}
	_, ok := columnKeyIn(tpl, "rows", 0)
	if ok {
		t.Fatal("want ok=false for empty value")
	}
}

func TestColumnKeyIn_NonMapOption(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "rows", Options: []any{"plain-string"}},
	}}
	_, ok := columnKeyIn(tpl, "rows", 0)
	if ok {
		t.Fatal("want ok=false for non-map option")
	}
}

func TestFacetField_Namespaces(t *testing.T) {
	if got := facetField("urgency"); got != "facet:urgency" {
		t.Fatalf("facetField = %q, want facet:urgency", got)
	}
}
