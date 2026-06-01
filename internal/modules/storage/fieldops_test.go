package storage

import (
	"context"
	"reflect"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

var bg = context.Background()

func strs(in []any) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = toStr(v)
	}
	return out
}

func TestSortListNatural(t *testing.T) {
	in := []any{"vwHPRMStudent", "vwHpRmAccount", "vwGebouw", "vwGebouw2", "vwGebouw10"}
	got := strs(sortList(in, false))
	want := []string{"vwGebouw", "vwGebouw2", "vwGebouw10", "vwHpRmAccount", "vwHPRMStudent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("asc natural sort\n got=%v\nwant=%v", got, want)
	}
	gotDesc := strs(sortList(in, true))
	for i := range want {
		if gotDesc[i] != want[len(want)-1-i] {
			t.Fatalf("desc not reverse of asc: %v vs %v", gotDesc, want)
		}
	}
}

func TestSortListIsStableCopy(t *testing.T) {
	in := []any{"b", "a"}
	_ = sortList(in, false)
	if in[0] != "b" || in[1] != "a" {
		t.Fatalf("input mutated: %v", in)
	}
}

func TestSortTableByColumnTypes(t *testing.T) {
	rows := []any{
		[]any{"beta", float64(10), "2024-01-02"},
		[]any{"alpha", float64(2), "2023-12-31"},
		[]any{"gamma", float64(100), "2024-03-01"},
	}
	// string column 0
	got0 := sortTable(rows, 0, "string", false)
	if toStr(cellAt(got0[0], 0)) != "alpha" || toStr(cellAt(got0[2], 0)) != "gamma" {
		t.Fatalf("string sort wrong: %v", got0)
	}
	// number column 1 (must beat lexical: 2 < 10 < 100)
	got1 := sortTable(rows, 1, "number", false)
	if toStr(cellAt(got1[0], 1)) != "2" || toStr(cellAt(got1[1], 1)) != "10" || toStr(cellAt(got1[2], 1)) != "100" {
		t.Fatalf("number sort wrong: %v", strs([]any{cellAt(got1[0], 1), cellAt(got1[1], 1), cellAt(got1[2], 1)}))
	}
	// date column 2
	got2 := sortTable(rows, 2, "date", false)
	if toStr(cellAt(got2[0], 2)) != "2023-12-31" || toStr(cellAt(got2[2], 2)) != "2024-03-01" {
		t.Fatalf("date sort wrong: %v", got2)
	}
}

func TestSortTableShortRowSortsAsEmpty(t *testing.T) {
	rows := []any{
		[]any{"b"},
		[]any{}, // missing cell -> empty, sorts first asc
		[]any{"a"},
	}
	got := sortTable(rows, 0, "string", false)
	if len(asSlice(got[0])) != 0 {
		t.Fatalf("short row should sort first asc: %v", got)
	}
}

func TestDedupList(t *testing.T) {
	in := []any{"a", "b", "a", "c", "b"}
	got := strs(dedupList(in))
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedup list\n got=%v\nwant=%v", got, want)
	}
}

func TestDedupTableByColumnKeepsFirst(t *testing.T) {
	rows := []any{
		[]any{"ReadEumAccountAll", "Direct"},
		[]any{"ReadEumAccountByPcn", "Direct"},
		[]any{"ReadEumAccountAll", "Indirect"}, // dup key in col 0
	}
	got := dedupTable(rows, 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d: %v", len(got), got)
	}
	if toStr(cellAt(got[0], 1)) != "Direct" {
		t.Fatalf("first occurrence not kept: %v", got)
	}
}

func TestCompareCellsFallback(t *testing.T) {
	// non-numeric in a number column falls back to natural string compare
	if compareCells("x", "y", "number") >= 0 {
		t.Fatalf("expected x < y on fallback")
	}
	// bool: false < true
	if compareCells(false, true, "bool") >= 0 {
		t.Fatalf("expected false < true")
	}
}

// ── Manager value-transform (schema from template, no disk I/O) ──────────

func col(value, typ string) any { return map[string]any{"value": value, "type": typ} }

func seedListTableTemplate(t *testing.T, tplM *template.Manager) {
	t.Helper()
	if err := tplM.SaveTemplate("ods.yaml", &template.Template{
		Name: "ods",
		Fields: []template.Field{
			{Key: "views", Type: "list"},
			{Key: "procs", Type: "table", Options: []any{
				col("name", "string"),
				col("kind", "string"),
			}},
			{Key: "title", Type: "text"},
		},
	}); err != nil {
		t.Fatalf("seed template: %v", err)
	}
}

func TestSortFieldValue_List(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	seedListTableTemplate(t, tplM)
	if r := m.SaveForm(bg, "ods.yaml", "rec", map[string]any{
		"views": []any{"vwB", "vwA", "vwC"}, "title": "keep me",
	}); !r.Success {
		t.Fatalf("seed: %v", r.Error)
	}
	out, err := m.SortFieldValue("ods.yaml", "rec", "views", "", "asc")
	if err != nil {
		t.Fatalf("sort failed: %v", err)
	}
	if got := strs(asSlice(out)); !reflect.DeepEqual(got, []string{"vwA", "vwB", "vwC"}) {
		t.Fatalf("sorted views = %v", got)
	}
	// Read-only: the on-disk record is untouched (save persists, not sort).
	if disk := strs(asSlice(m.LoadForm("ods.yaml", "rec").Data["views"])); disk[0] != "vwB" {
		t.Fatalf("sort must not write disk, got %v", disk)
	}
}

func TestSortFieldValue_TableByNamedColumnDesc(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	seedListTableTemplate(t, tplM)
	m.SaveForm(bg, "ods.yaml", "rec", map[string]any{"procs": []any{
		[]any{"Bravo", "Direct"},
		[]any{"Alpha", "Indirect"},
		[]any{"Charlie", "Direct"},
	}})
	out, err := m.SortFieldValue("ods.yaml", "rec", "procs", "name", "desc")
	if err != nil {
		t.Fatalf("sort failed: %v", err)
	}
	rows := asSlice(out)
	if toStr(cellAt(rows[0], 0)) != "Charlie" || toStr(cellAt(rows[2], 0)) != "Alpha" {
		t.Fatalf("desc table sort wrong: %v", rows)
	}
}

func TestDedupFieldValue_TableByColumn(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	seedListTableTemplate(t, tplM)
	m.SaveForm(bg, "ods.yaml", "rec", map[string]any{"procs": []any{
		[]any{"ReadAll", "Direct"},
		[]any{"ReadByPcn", "Direct"},
		[]any{"ReadAll", "Indirect"},
	}})
	out, err := m.DedupFieldValue("ods.yaml", "rec", "procs", "name")
	if err != nil {
		t.Fatalf("dedup failed: %v", err)
	}
	if rows := asSlice(out); len(rows) != 2 {
		t.Fatalf("expected 2 rows after dedup, got %d: %v", len(rows), rows)
	}
}

func TestFieldOps_UnhappyPaths(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	seedListTableTemplate(t, tplM)
	m.SaveForm(bg, "ods.yaml", "rec", map[string]any{"views": []any{"a"}})

	if _, err := m.SortFieldValue("ods.yaml", "rec", "nope", "", "asc"); err == nil {
		t.Fatalf("expected unknown-field error")
	}
	if _, err := m.SortFieldValue("ods.yaml", "missing", "views", "", "asc"); err == nil {
		t.Fatalf("expected missing-record error")
	}
	if _, err := m.SortFieldValue("ods.yaml", "rec", "title", "", "asc"); err == nil {
		t.Fatalf("expected not-sortable error for text field")
	}
	if _, err := m.SortFieldValue("ods.yaml", "rec", "procs", "ghost", "asc"); err == nil {
		t.Fatalf("expected unknown-column error")
	}
}
