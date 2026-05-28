package query

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// fakeIndex records the calls the Manager makes and returns canned data,
// so these tests pin the translation + result-shaping the query module
// owns; the SQL correctness is covered in the index package.
type fakeIndex struct {
	projectSpec index.ProjectSpec
	projectTpl  string
	projectRows []index.ProjectRow
	projectErr  error

	aggDims    []index.AggDim
	aggFilters []index.AggFilter
	aggRows    []index.StatRawRow
	aggErr     error

	total int
}

func (f *fakeIndex) ProjectRows(tpl string, spec index.ProjectSpec) ([]index.ProjectRow, error) {
	f.projectTpl = tpl
	f.projectSpec = spec
	return f.projectRows, f.projectErr
}

func (f *fakeIndex) AggregateRaw(tpl string, dims []index.AggDim, nums []index.AggNum, filters []index.AggFilter) ([]index.StatRawRow, error) {
	f.aggDims = dims
	f.aggFilters = filters
	return f.aggRows, f.aggErr
}

func (f *fakeIndex) TotalForms(string) (int, error) { return f.total, nil }

func pcell(text string, num *float64) index.ProjectCell {
	c := index.ProjectCell{Text: text}
	if num != nil {
		c.Num = sql.NullFloat64{Float64: *num, Valid: true}
	}
	return c
}

func TestRun_EmptyTemplate(t *testing.T) {
	m := NewManager(&fakeIndex{})
	if _, err := m.Run(Spec{}); err == nil {
		t.Fatal("expected error for empty template")
	}
}

func TestRun_RowListing(t *testing.T) {
	n10 := 10.0
	fx := &fakeIndex{
		total: 42,
		projectRows: []index.ProjectRow{
			{Form: "a.meta.json", Cells: []index.ProjectCell{pcell("high", nil), pcell("10", &n10)}},
		},
	}
	m := NewManager(fx)
	res, err := m.Run(Spec{
		Template: "basic.yaml",
		Columns: []Column{
			{Header: "Status", Source: Source{Kind: "field", Key: "status"}},
			{Header: "Amount", Source: Source{Kind: "field", Key: "amount"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fx.projectTpl != "basic.yaml" {
		t.Errorf("template not forwarded: %q", fx.projectTpl)
	}
	if len(res.Columns) != 2 || res.Columns[0] != "Status" || res.Columns[1] != "Amount" {
		t.Errorf("headers = %v", res.Columns)
	}
	if res.Count != 1 || res.Total != 42 {
		t.Errorf("count/total = %d/%d, want 1/42", res.Count, res.Total)
	}
	if res.Rows[0][1].Num == nil || *res.Rows[0][1].Num != 10 {
		t.Errorf("numeric cell not typed: %+v", res.Rows[0][1])
	}
}

func TestRun_DistinctPassthrough(t *testing.T) {
	fx := &fakeIndex{}
	m := NewManager(fx)
	_, err := m.Run(Spec{
		Template: "basic.yaml",
		Columns:  []Column{{Header: "Tag", Source: Source{Kind: "field", Key: "tags"}}},
		Distinct: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fx.projectSpec.Distinct {
		t.Error("Distinct not forwarded to ProjectSpec")
	}
}

func TestRun_FilterTranslation(t *testing.T) {
	fx := &fakeIndex{}
	m := NewManager(fx)
	_, err := m.Run(Spec{
		Template: "basic.yaml",
		Columns:  []Column{{Header: "Status", Source: Source{Kind: "field", Key: "status"}}},
		Filters:  []Filter{{Source: Source{Kind: "field", Key: "status"}, Op: "eq", Value: "open"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fx.projectSpec.Filters) != 1 {
		t.Fatalf("filters not forwarded: %+v", fx.projectSpec.Filters)
	}
	got := fx.projectSpec.Filters[0]
	if got.Key != "status" || got.Op != "eq" || got.Value != "open" {
		t.Errorf("filter translated wrong: %+v", got)
	}
}

func TestRun_OrderColumnOutOfRange(t *testing.T) {
	m := NewManager(&fakeIndex{})
	_, err := m.Run(Spec{
		Template: "basic.yaml",
		Columns:  []Column{{Header: "Status", Source: Source{Kind: "field", Key: "status"}}},
		OrderBy:  []Sort{{Column: 5}},
	})
	if err == nil {
		t.Fatal("expected out-of-range order error")
	}
}

func TestRun_GroupCount(t *testing.T) {
	fx := &fakeIndex{
		total: 3,
		aggRows: []index.StatRawRow{
			{Form: "a", Dims: []string{"high"}},
			{Form: "b", Dims: []string{"low"}},
			{Form: "c", Dims: []string{"high"}},
		},
	}
	m := NewManager(fx)
	res, err := m.Run(Spec{
		Template: "basic.yaml",
		Columns:  []Column{{Header: "Status", Source: Source{Kind: "field", Key: "status"}}},
		GroupBy:  []int{0},
		Count:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// dim forwarded
	if len(fx.aggDims) != 1 || fx.aggDims[0].Key != "status" {
		t.Errorf("group dim not forwarded: %+v", fx.aggDims)
	}
	// headers = group col + count
	if len(res.Columns) != 2 || res.Columns[0] != "Status" || res.Columns[1] != "count" {
		t.Errorf("group headers = %v", res.Columns)
	}
	// two groups, ordered by key: high(2), low(1)
	if len(res.Rows) != 2 {
		t.Fatalf("got %d groups, want 2", len(res.Rows))
	}
	byKey := map[string]float64{}
	for _, r := range res.Rows {
		if r[1].Num == nil {
			t.Fatalf("count cell not numeric: %+v", r[1])
		}
		byKey[r[0].Text] = *r[1].Num
	}
	if byKey["high"] != 2 || byKey["low"] != 1 {
		t.Errorf("group counts = %v, want high:2 low:1", byKey)
	}
}

func TestRun_GroupColumnOutOfRange(t *testing.T) {
	m := NewManager(&fakeIndex{})
	_, err := m.Run(Spec{
		Template: "basic.yaml",
		Columns:  []Column{{Header: "Status", Source: Source{Kind: "field", Key: "status"}}},
		GroupBy:  []int{3},
	})
	if err == nil {
		t.Fatal("expected out-of-range group error")
	}
}

func TestRun_CustomCountHeader(t *testing.T) {
	fx := &fakeIndex{aggRows: []index.StatRawRow{{Form: "a", Dims: []string{"x"}}}}
	m := NewManager(fx)
	res, err := m.Run(Spec{
		Template:    "basic.yaml",
		Columns:     []Column{{Header: "K", Source: Source{Kind: "field", Key: "k"}}},
		GroupBy:     []int{0},
		Count:       true,
		CountHeader: "records",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Columns[1] != "records" {
		t.Errorf("count header = %q, want records", res.Columns[1])
	}
}

func TestRun_ProjectErrorPropagates(t *testing.T) {
	fx := &fakeIndex{projectErr: errors.New("boom")}
	m := NewManager(fx)
	if _, err := m.Run(Spec{
		Template: "basic.yaml",
		Columns:  []Column{{Header: "S", Source: Source{Kind: "field", Key: "s"}}},
	}); err == nil {
		t.Fatal("expected propagated project error")
	}
}
