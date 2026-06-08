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
