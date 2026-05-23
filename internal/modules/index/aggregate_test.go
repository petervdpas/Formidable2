package index

import (
	"strconv"
	"testing"
)

// seedValuesDB builds a small store: one template "basic.yaml" with
// three forms carrying a numeric "amount", a "status" dropdown, a
// "due" date, a "prio" facet, and a two-column table "items" (text
// col0, number col1).
func seedValuesDB(t *testing.T) *Manager {
	t.Helper()
	db := openTestDB(t)

	ftoa := func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
	mk := func(file string, amount float64, status, due string, qty float64) FormRow {
		col0, col1 := 0, 1
		a, q := amount, qty
		dr, _ := dateRow("due", nil, due)
		return FormRow{
			Template: "basic.yaml", Filename: file, Mtime: 100,
			Facets: []FormFacet{{Key: "prio", Set: true, Selected: status}},
			Values: []FormValueRow{
				{FieldKey: "amount", ValueType: "number", Num: &a, Text: ftoa(amount)},
				{FieldKey: "status", ValueType: "text", Text: status},
				dr,
				{FieldKey: "items", Col: &col0, ValueType: "text", Text: "row-" + file},
				{FieldKey: "items", Col: &col1, ValueType: "number", Num: &q, Text: ftoa(qty)},
			},
		}
	}

	must(t, Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100)},
		UpsertForms: []FormRow{
			mk("a.meta.json", 10, "high", "2026-01-15", 2),
			mk("b.meta.json", 20, "low", "2026-01-20", 3),
			mk("c.meta.json", 30, "high", "2026-02-05", 5),
		},
	}))
	return &Manager{db: db}
}

func TestTotalForms(t *testing.T) {
	m := seedValuesDB(t)
	n, err := m.TotalForms("basic.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("TotalForms = %d, want 3", n)
	}
}

func TestValueDistribution_Scalar(t *testing.T) {
	m := seedValuesDB(t)
	buckets, err := m.ValueDistribution("basic.yaml", "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, b := range buckets {
		got[b.Label] = b.Count
	}
	if got["high"] != 2 || got["low"] != 1 {
		t.Errorf("status distribution = %v, want high:2 low:1", got)
	}
}

func TestNumericValues_ScalarField(t *testing.T) {
	m := seedValuesDB(t)
	vals, err := m.NumericValues("basic.yaml", "amount", nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	if len(vals) != 3 || sum != 60 {
		t.Errorf("amount values = %v (sum %v), want 3 values summing 60", vals, sum)
	}
}

func TestNumericValues_TableColumn(t *testing.T) {
	m := seedValuesDB(t)
	col1 := 1
	vals, err := m.NumericValues("basic.yaml", "items", &col1)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	if len(vals) != 3 || sum != 10 {
		t.Errorf("items col1 values = %v (sum %v), want 3 summing 10", vals, sum)
	}
}

func TestFacetDistribution(t *testing.T) {
	m := seedValuesDB(t)
	buckets, err := m.FacetDistribution("basic.yaml", "prio")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, b := range buckets {
		got[b.Label] = b.Count
	}
	if got["high"] != 2 || got["low"] != 1 {
		t.Errorf("prio facet distribution = %v, want high:2 low:1", got)
	}
}

func TestDateSeries_ByMonth(t *testing.T) {
	m := seedValuesDB(t)
	buckets, err := m.DateSeries("basic.yaml", "due", nil, "month")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, b := range buckets {
		got[b.Label] = b.Count
	}
	if got["2026-01"] != 2 || got["2026-02"] != 1 {
		t.Errorf("due by month = %v, want 2026-01:2 2026-02:1", got)
	}
}
