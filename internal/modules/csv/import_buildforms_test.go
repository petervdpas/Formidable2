package csv

import (
	"reflect"
	"testing"
)

func tableField(key string, cols ...string) FieldSpec {
	opts := make([]any, len(cols))
	for i, c := range cols {
		opts[i] = map[string]any{"value": c, "label": c}
	}
	return FieldSpec{Key: key, Type: "table", Options: opts}
}

func TestBuildImportForms_Unaligned_OneFormPerRow(t *testing.T) {
	fields := []FieldSpec{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}}
	plan := ImportPlan{Columns: []ImportColumn{
		{Header: "id", Target: "id"},
		{Header: "name", Target: "name"},
	}}
	headers := []string{"id", "name"}
	rows := [][]string{{"g1", "Alice"}, {"g2", "Bob"}}
	got := BuildImportForms(plan, headers, rows, fields)
	if len(got) != 2 {
		t.Fatalf("want 2 forms, got %d", len(got))
	}
	if got[0].Data["id"] != "g1" || got[0].Data["name"] != "Alice" {
		t.Errorf("form0 = %+v", got[0].Data)
	}
}

func TestBuildImportForms_AlignedTable_RegroupsRows(t *testing.T) {
	fields := []FieldSpec{
		{Key: "id", Type: "guid"},
		{Key: "name", Type: "text"},
		tableField("gap", "onderdeel", "actie"),
	}
	plan := ImportPlan{
		AlignSource: "gap",
		GroupKey:    "id",
		Columns: []ImportColumn{
			{Header: "id", Target: "id"},
			{Header: "name", Target: "name"},
			{Header: "gap-onderdeel", Target: "gap.onderdeel"},
			{Header: "gap-actie", Target: "gap.actie"},
		},
	}
	headers := []string{"id", "name", "gap-onderdeel", "gap-actie"}
	rows := [][]string{
		{"g1", "Alice", "Governance", "DOR afstemmen"},
		{"g1", "Alice", "CIB", "Workflow integreren"},
		{"g2", "Bob", "Proceservaring", "Review"},
	}
	got := BuildImportForms(plan, headers, rows, fields)
	if len(got) != 2 {
		t.Fatalf("want 2 grouped forms, got %d", len(got))
	}
	// g1 has two gap rows, scalars taken once.
	g1 := got[0]
	if g1.Key != "g1" || g1.Data["id"] != "g1" || g1.Data["name"] != "Alice" {
		t.Errorf("g1 scalars wrong: key=%q data=%+v", g1.Key, g1.Data)
	}
	gap, ok := g1.Data["gap"].([]any)
	if !ok || len(gap) != 2 {
		t.Fatalf("g1 gap = %#v, want 2 rows", g1.Data["gap"])
	}
	if !reflect.DeepEqual(gap[0], []any{"Governance", "DOR afstemmen"}) {
		t.Errorf("g1 gap[0] = %#v", gap[0])
	}
	if !reflect.DeepEqual(gap[1], []any{"CIB", "Workflow integreren"}) {
		t.Errorf("g1 gap[1] = %#v", gap[1])
	}
	g2gap, _ := got[1].Data["gap"].([]any)
	if len(g2gap) != 1 {
		t.Errorf("g2 gap want 1 row, got %#v", got[1].Data["gap"])
	}
}

// The round-trip contract: export an aligned form, import it back, and
// the reconstructed entry matches the original data.
func TestExportThenImport_RoundTripsAlignedTable(t *testing.T) {
	fields := []FieldSpec{
		{Key: "id", Type: "guid"},
		{Key: "name", Type: "text"},
		tableField("gap", "onderdeel", "actie"),
	}
	original := map[string]any{
		"id":   "g1",
		"name": "Alice",
		"gap": []any{
			[]any{"Governance", "DOR afstemmen"},
			[]any{"CIB", "Workflow integreren"},
		},
	}

	exportPlan := ExportPlan{
		AlignSource: "gap",
		Columns: []ExportColumn{
			{Header: "id", SourceKeys: []string{"id"}},
			{Header: "name", SourceKeys: []string{"name"}},
			{Header: "gap.onderdeel", SourceKeys: []string{"gap.onderdeel"}},
			{Header: "gap.actie", SourceKeys: []string{"gap.actie"}},
		},
	}
	exported := BuildExportRows(exportPlan, []map[string]any{original}, fields)
	// Header + 2 data rows.
	if len(exported) != 3 {
		t.Fatalf("export rows = %d, want 3", len(exported))
	}

	headers := exported[0]
	dataRows := exported[1:]
	importPlan := ImportPlan{
		AlignSource: "gap",
		GroupKey:    "id",
		Columns: []ImportColumn{
			{Header: "id", Target: "id"},
			{Header: "name", Target: "name"},
			{Header: "gap.onderdeel", Target: "gap.onderdeel"},
			{Header: "gap.actie", Target: "gap.actie"},
		},
	}
	got := BuildImportForms(importPlan, headers, dataRows, fields)
	if len(got) != 1 {
		t.Fatalf("import forms = %d, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0].Data, original) {
		t.Errorf("round-trip mismatch:\n got = %#v\nwant = %#v", got[0].Data, original)
	}
}

func TestBuildImportForms_AlignedList_RegroupsToListItems(t *testing.T) {
	// A list-typed align source regroups rows by group key and rebuilds
	// the aligned field as plain string list items (exercises listItem).
	fields := []FieldSpec{
		{Key: "id", Type: "guid"},
		{Key: "name", Type: "text"},
		{Key: "phones", Type: "list"},
	}
	plan := ImportPlan{
		AlignSource: "phones",
		GroupKey:    "id",
		Columns: []ImportColumn{
			{Header: "id", Target: "id"},
			{Header: "name", Target: "name"},
			{Header: "phone", Target: "phones"},
		},
	}
	headers := []string{"id", "name", "phone"}
	rows := [][]string{
		{"g1", "Alice", "555-1"},
		{"g1", "Alice", "555-2"},
		{"g2", "Bob", "555-9"},
	}
	got := BuildImportForms(plan, headers, rows, fields)
	if len(got) != 2 {
		t.Fatalf("want 2 grouped forms, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].Data["phones"], []any{"555-1", "555-2"}) {
		t.Errorf("g1 phones = %#v, want two list items", got[0].Data["phones"])
	}
	if !reflect.DeepEqual(got[1].Data["phones"], []any{"555-9"}) {
		t.Errorf("g2 phones = %#v", got[1].Data["phones"])
	}
}

func TestBuildImportForms_AlignedTable_TypedColumnsCoerced(t *testing.T) {
	// Table column options carrying a "type" coerce their cells: number
	// columns become float64, bool columns become bool (exercises coerceCell).
	fields := []FieldSpec{
		{Key: "id", Type: "guid"},
		{Key: "tbl", Type: "table", Options: []any{
			map[string]any{"value": "qty", "label": "Qty", "type": "number"},
			map[string]any{"value": "active", "label": "Active", "type": "bool"},
		}},
	}
	plan := ImportPlan{
		AlignSource: "tbl",
		GroupKey:    "id",
		Columns: []ImportColumn{
			{Header: "id", Target: "id"},
			{Header: "qty", Target: "tbl.qty"},
			{Header: "active", Target: "tbl.active"},
		},
	}
	headers := []string{"id", "qty", "active"}
	rows := [][]string{{"g1", "42", "yes"}}
	got := BuildImportForms(plan, headers, rows, fields)
	if len(got) != 1 {
		t.Fatalf("want 1 form, got %d", len(got))
	}
	tbl, ok := got[0].Data["tbl"].([]any)
	if !ok || len(tbl) != 1 {
		t.Fatalf("tbl = %#v, want one row", got[0].Data["tbl"])
	}
	cells, ok := tbl[0].([]any)
	if !ok || len(cells) != 2 {
		t.Fatalf("cells = %#v, want 2", tbl[0])
	}
	if cells[0] != float64(42) {
		t.Errorf("qty cell = %#v, want float64(42)", cells[0])
	}
	if cells[1] != true {
		t.Errorf("active cell = %#v, want true", cells[1])
	}
}
