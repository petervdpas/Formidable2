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
