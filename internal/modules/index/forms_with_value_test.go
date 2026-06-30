package index

import (
	"path/filepath"
	"sort"
	"testing"
)

// TestFormsWithValue covers the narrowing query the datacore planner pushes
// down, including its edges: it matches scalar values only (col IS NULL), so a
// table-column cell with the same text is never returned; a missing field or
// missing template yields an empty slice, not an error.
func TestFormsWithValue(t *testing.T) {
	idxM, err := NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	col0 := 0
	forms := []FormRow{
		{Template: "basic.yaml", Filename: "a.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "region", ValueType: "text", Text: "east"},
			{FieldKey: "items", Col: &col0, ValueType: "text", Text: "east"}, // same text, table column
		}},
		{Template: "basic.yaml", Filename: "b.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "region", ValueType: "text", Text: "west"},
		}},
		{Template: "basic.yaml", Filename: "c.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "region", ValueType: "text", Text: "east"},
		}},
	}
	if err := Reconcile(idxM.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Scalar match: a and c carry region=east. The table column "items"=east on
	// a must NOT add a duplicate or pull in by the col guard.
	got, err := idxM.FormsWithValue("basic.yaml", "region", "east")
	if err != nil {
		t.Fatalf("FormsWithValue region=east: %v", err)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "a.meta.json" || got[1] != "c.meta.json" {
		t.Fatalf("region=east = %v, want [a c]", got)
	}

	// Table-column value must not match through the scalar query.
	if rows, err := idxM.FormsWithValue("basic.yaml", "items", "east"); err != nil || len(rows) != 0 {
		t.Fatalf("items=east = %v err=%v, want empty (col IS NULL guard)", rows, err)
	}

	// Missing field and missing template: empty, not an error.
	if rows, err := idxM.FormsWithValue("basic.yaml", "nosuchfield", "x"); err != nil || len(rows) != 0 {
		t.Fatalf("missing field = %v err=%v, want empty", rows, err)
	}
	if rows, err := idxM.FormsWithValue("absent.yaml", "region", "east"); err != nil || len(rows) != 0 {
		t.Fatalf("missing template = %v err=%v, want empty", rows, err)
	}
}

// TestMaxValue covers the auto-assign read: the greatest scalar num_value for a
// field, ok=false when there are none, scalar-only (a table-column cell with a
// larger number must not win through the col guard).
func TestMaxValue(t *testing.T) {
	idxM, err := NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	num := func(n float64) *float64 { return &n }
	col0 := 0
	forms := []FormRow{
		{Template: "deck.yaml", Filename: "a.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "pos", ValueType: "number", Num: num(10), Text: "10"},
		}},
		{Template: "deck.yaml", Filename: "b.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "pos", ValueType: "number", Num: num(30), Text: "30"},
			// A table-column cell with a bigger number must not win (col IS NULL guard).
			{FieldKey: "pos", Col: &col0, ValueType: "number", Num: num(999), Text: "999"},
		}},
		{Template: "deck.yaml", Filename: "c.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "pos", ValueType: "number", Num: num(20), Text: "20"},
		}},
	}
	if err := Reconcile(idxM.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "deck.yaml", Name: "deck", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got, ok, err := idxM.MaxValue("deck.yaml", "pos")
	if err != nil || !ok {
		t.Fatalf("MaxValue pos: got=%v ok=%v err=%v", got, ok, err)
	}
	if got != 30 {
		t.Errorf("MaxValue pos = %v, want 30 (scalar max, not the 999 table cell)", got)
	}

	// A field with no rows yields ok=false, not an error.
	if v, ok, err := idxM.MaxValue("deck.yaml", "nosuch"); err != nil || ok {
		t.Errorf("MaxValue missing field = %v ok=%v err=%v, want ok=false", v, ok, err)
	}
}

// TestFormsWithValueOp covers the api-field filter narrowing: eq/ne over
// text_value and gt/ge/lt/le over num_value, scalar-only (col IS NULL).
func TestFormsWithValueOp(t *testing.T) {
	idxM, err := NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	num := func(n float64) *float64 { return &n }
	forms := []FormRow{
		{Template: "basic.yaml", Filename: "a.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "region", ValueType: "text", Text: "east"},
			{FieldKey: "amount", ValueType: "number", Num: num(100), Text: "100"},
		}},
		{Template: "basic.yaml", Filename: "b.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "region", ValueType: "text", Text: "west"},
			{FieldKey: "amount", ValueType: "number", Num: num(200), Text: "200"},
		}},
		{Template: "basic.yaml", Filename: "c.meta.json", Mtime: 100, Values: []FormValueRow{
			{FieldKey: "region", ValueType: "text", Text: "east"},
			{FieldKey: "amount", ValueType: "number", Num: num(300), Text: "300"},
		}},
	}
	if err := Reconcile(idxM.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "basic.yaml", Name: "basic", Mtime: 100}},
		UpsertForms:     forms,
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	check := func(op, field, val string, want ...string) {
		t.Helper()
		got, err := idxM.FormsWithValueOp("basic.yaml", field, op, val)
		if err != nil {
			t.Fatalf("FormsWithValueOp %s %s %s: %v", field, op, val, err)
		}
		sort.Strings(got)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("%s %s %s = %v, want %v", field, op, val, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s %s %s = %v, want %v", field, op, val, got, want)
			}
		}
	}

	check("eq", "region", "east", "a.meta.json", "c.meta.json")
	check("ne", "region", "east", "b.meta.json")
	check("ge", "amount", "200", "b.meta.json", "c.meta.json")
	check("gt", "amount", "200", "c.meta.json")
	check("lt", "amount", "200", "a.meta.json")
	check("le", "amount", "100", "a.meta.json")

	if _, err := idxM.FormsWithValueOp("basic.yaml", "amount", "gt", "abc"); err == nil {
		t.Error("non-numeric compare value should error")
	}
	if _, err := idxM.FormsWithValueOp("basic.yaml", "region", "contains", "x"); err == nil {
		t.Error("invalid op should error")
	}
}

// TestEmptyIndex_FormsWithValueEmpty: scalar-value lookup over an empty index
// returns no filenames.
func TestEmptyIndex_FormsWithValueEmpty(t *testing.T) {
	m := newEmptyManager(t)
	got, err := m.FormsWithValue("basic.yaml", "status", "open")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("FormsWithValue on empty index = %v, want empty", got)
	}
}
