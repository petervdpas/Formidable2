package query

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

type fakeLoader struct {
	tpl   *template.Template
	forms []FormData
	err   error
}

func (f fakeLoader) Template(string) (*template.Template, error) { return f.tpl, f.err }
func (f fakeLoader) Forms(string) ([]FormData, error)            { return f.forms, f.err }

func tableField(key string, opts ...[2]string) template.Field {
	o := make([]any, len(opts))
	for i, p := range opts {
		o[i] = map[string]any{"value": p[0], "type": p[1], "label": p[0]}
	}
	return template.Field{Key: key, Type: "table", Options: o}
}

// appsTpl: a scalar "team", a facet "tier", and a table "apps" with a
// string "name" column and a number "cost" column.
func appsTpl() *template.Template {
	return &template.Template{
		Facets: []template.Facet{{Key: "tier"}},
		Fields: []template.Field{
			{Key: "team", Type: "text"},
			tableField("apps", [2]string{"name", "string"}, [2]string{"cost", "number"}),
		},
	}
}

func TestPrepare_ScalarAndFacetOnePerForm(t *testing.T) {
	l := fakeLoader{
		tpl: appsTpl(),
		forms: []FormData{
			{Filename: "a.json", Data: map[string]any{"team": "Alpha"}, Facets: map[string]string{"tier": "gold"}},
			{Filename: "b.json", Data: map[string]any{"team": "Beta"}, Facets: map[string]string{"tier": "silver"}},
		},
	}
	m, err := Prepare(Spec{Template: "t.yaml", Columns: cols(scalar("team"), facet("tier"))}, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Rows) != 2 || m.FormCount != 2 {
		t.Fatalf("want 2 rows / FormCount 2, got %d / %d", len(m.Rows), m.FormCount)
	}
	res, _ := m.Execute(Spec{Columns: cols(scalar("team"), facet("tier"))})
	if res.Rows[0][0].Text != "Alpha" || res.Rows[0][1].Text != "gold" {
		t.Fatalf("row 0 wrong: %v", res.Rows[0])
	}
	if res.Total != 2 {
		t.Fatalf("Total = %d, want 2", res.Total)
	}
}

func TestPrepare_SingleTableColumnsAligned(t *testing.T) {
	l := fakeLoader{
		tpl: appsTpl(),
		forms: []FormData{{
			Filename: "a.json",
			Data: map[string]any{"apps": []any{
				[]any{"billing", 100.0},
				[]any{"search", 40.0},
			}},
		}},
	}
	spec := Spec{Template: "t.yaml", Columns: cols(tcol("apps", 0), tcol("apps", 1))}
	m, err := Prepare(spec, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(m.Rows))
	}
	// name aligns with cost on the same table row, and the hint comes from
	// the column type (cost is number).
	res, _ := m.Execute(spec)
	pairs := map[string]string{}
	for _, r := range res.Rows {
		pairs[r[0].Text] = r[1].Text
	}
	if pairs["billing"] != "100" || pairs["search"] != "40" {
		t.Fatalf("misaligned: %v", pairs)
	}
	// Provenance: row index + a non-empty content hash, count = 2.
	for _, row := range m.Rows {
		o := row.Origins[0]
		if o.Field != "apps" || o.Count != 2 || o.Hash == "" {
			t.Fatalf("bad origin: %+v", o)
		}
	}
}

func TestPrepare_IdenticalRowsSameHashDistinctByPosition(t *testing.T) {
	l := fakeLoader{
		tpl: appsTpl(),
		forms: []FormData{{
			Filename: "a.json",
			Data: map[string]any{"apps": []any{
				[]any{"dup", 5.0},
				[]any{"dup", 5.0},
			}},
		}},
	}
	spec := Spec{Template: "t.yaml", Columns: cols(tcol("apps", 0), tcol("apps", 1))}
	m, _ := Prepare(spec, l)
	if m.Rows[0].Origins[0].Hash != m.Rows[1].Origins[0].Hash {
		t.Fatal("identical content should hash equal")
	}
	if m.Rows[0].Origins[0].Row == m.Rows[1].Origins[0].Row {
		t.Fatal("duplicate rows must keep distinct positions")
	}
	// A sum over cost must count both duplicate rows: 5 + 5 = 10.
	res, _ := m.Execute(Spec{
		Columns:  cols(tcol("apps", 0)),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "sum", Source: tcol("apps", 1), Header: "s"}},
	})
	if res.Rows[0][1].Text != "10" {
		t.Fatalf("duplicate rows should sum to 10, got %s", res.Rows[0][1].Text)
	}
}

func TestPrepare_TwoTablesCartesianHonestSum(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		tableField("apps", [2]string{"name", "string"}, [2]string{"cost", "number"}),
		tableField("tasks", [2]string{"task", "string"}),
	}}
	l := fakeLoader{tpl: tpl, forms: []FormData{{
		Filename: "f.json",
		Data: map[string]any{
			"apps":  []any{[]any{"billing", 100.0}, []any{"search", 40.0}},
			"tasks": []any{[]any{"t1"}, []any{"t2"}, []any{"t3"}},
		},
	}}}
	spec := Spec{Template: "t.yaml", Columns: cols(tcol("apps", 0), tcol("apps", 1), tcol("tasks", 0))}
	m, err := Prepare(spec, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Rows) != 6 {
		t.Fatalf("want 6 cartesian rows (2x3), got %d", len(m.Rows))
	}
	// Group by task, sum apps.cost: each task sees both apps once -> 140,
	// not inflated by the cross-product.
	res, _ := m.Execute(Spec{
		Columns:  cols(tcol("tasks", 0), tcol("apps", 1)),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "sum", Source: tcol("apps", 1), Header: "s"}},
	})
	for _, r := range res.Rows {
		if r[1].Text != "140" {
			t.Fatalf("task %s sum=%s, want 140", r[0].Text, r[1].Text)
		}
	}
}

func TestPrepare_ListFieldFans(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{{Key: "tags", Type: "tags"}}}
	l := fakeLoader{tpl: tpl, forms: []FormData{{
		Filename: "a.json",
		Data:     map[string]any{"tags": []any{"x", "y", "z"}},
	}}}
	spec := Spec{Template: "t.yaml", Columns: cols(scalar("tags"))}
	m, _ := Prepare(spec, l)
	if len(m.Rows) != 3 {
		t.Fatalf("list of 3 should fan to 3 rows, got %d", len(m.Rows))
	}
	if m.Rows[0].Origins[0].Field != "tags" || m.Rows[0].Origins[0].Hash == "" {
		t.Fatalf("list origin missing: %+v", m.Rows[0].Origins)
	}
}

func TestPrepare_EmptyReferencedTableDropsForm(t *testing.T) {
	l := fakeLoader{
		tpl: appsTpl(),
		forms: []FormData{
			{Filename: "a.json", Data: map[string]any{"apps": []any{[]any{"x", 1.0}}}},
			{Filename: "b.json", Data: map[string]any{"apps": []any{}}}, // empty table
			{Filename: "c.json", Data: map[string]any{}},                // missing entirely
		},
	}
	spec := Spec{Template: "t.yaml", Columns: cols(tcol("apps", 0))}
	m, _ := Prepare(spec, l)
	if len(m.Rows) != 1 {
		t.Fatalf("only the form with a row should appear, got %d rows", len(m.Rows))
	}
	// FormCount still counts all three forms (the denominator).
	if m.FormCount != 3 {
		t.Fatalf("FormCount = %d, want 3", m.FormCount)
	}
}

func TestPrepare_DateHintFeedsAnomalyCheck(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		tableField("events", [2]string{"when", "date"}),
	}}
	l := fakeLoader{tpl: tpl, forms: []FormData{{
		Filename: "a.json",
		Data: map[string]any{"events": []any{
			[]any{"2026-05-29"},
			[]any{"nonsense"},
		}},
	}}}
	spec := Spec{Template: "t.yaml", Columns: cols(tcol("events", 0))}
	m, _ := Prepare(spec, l)
	res, _ := m.Execute(spec)
	if len(res.Anomalies) != 1 || res.Anomalies[0].Expected != "date" {
		t.Fatalf("want 1 date anomaly, got %+v", res.Anomalies)
	}
}

func TestPrepare_NilTemplateErrors(t *testing.T) {
	if _, err := Prepare(Spec{Template: "missing.yaml"}, fakeLoader{tpl: nil}); err == nil {
		t.Fatal("want error for missing template")
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
