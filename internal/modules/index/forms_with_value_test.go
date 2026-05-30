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
