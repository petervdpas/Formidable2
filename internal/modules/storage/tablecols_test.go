package storage

import (
	"context"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// A table field with three columns whose cells are stored positionally.
func newTableStack(t *testing.T, columns ...string) (*Manager, *template.Manager) {
	t.Helper()
	m, _, tplM, _ := newTestStack(t)
	opts := make([]any, 0, len(columns))
	for _, c := range columns {
		opts = append(opts, map[string]any{"value": c, "type": "string", "label": c})
	}
	if err := tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "tbl", Type: "table", Options: opts}},
	}); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	return m, tplM
}

func loadRows(t *testing.T, m *Manager, df string) []any {
	t.Helper()
	f := m.LoadForm("t.yaml", df)
	if f == nil {
		t.Fatalf("load %s: nil", df)
	}
	rows, _ := f.Data["tbl"].([]any)
	return rows
}

func TestRemapTableColumns_Reorder(t *testing.T) {
	m, _ := newTableStack(t, "a", "b", "c")
	r := m.SaveForm(context.Background(), "t.yaml", "r1.json", map[string]any{
		"tbl": []any{[]any{"a1", "b1", "c1"}},
	})
	if !r.Success {
		t.Fatalf("seed record: %+v", r)
	}

	// New order [a, c, b] -> perm maps new positions to old indices.
	n, err := m.RemapTableColumns("t.yaml", "tbl", []int{0, 2, 1})
	if err != nil || n != 1 {
		t.Fatalf("remap: n=%d err=%v", n, err)
	}
	row := loadRows(t, m, "r1.json")[0].([]any)
	if row[0] != "a1" || row[1] != "c1" || row[2] != "b1" {
		t.Errorf("reordered row = %v, want [a1 c1 b1]", row)
	}
}

func TestRemapTableColumns_DropMiddle(t *testing.T) {
	m, _ := newTableStack(t, "a", "b", "c")
	m.SaveForm(context.Background(), "t.yaml", "r1.json", map[string]any{
		"tbl": []any{[]any{"a1", "b1", "c1"}, []any{"a2", "b2", "c2"}},
	})

	// Column "b" dropped: new order is [a, c] -> old indices [0, 2].
	n, err := m.RemapTableColumns("t.yaml", "tbl", []int{0, 2})
	if err != nil || n != 1 {
		t.Fatalf("remap: n=%d err=%v", n, err)
	}
	rows := loadRows(t, m, "r1.json")
	r0 := rows[0].([]any)
	r1 := rows[1].([]any)
	if len(r0) != 2 || r0[0] != "a1" || r0[1] != "c1" {
		t.Errorf("row0 = %v, want [a1 c1]", r0)
	}
	if len(r1) != 2 || r1[0] != "a2" || r1[1] != "c2" {
		t.Errorf("row1 = %v, want [a2 c2]", r1)
	}
}

func TestRemapTableColumns_InsertMiddleBlanksNewCell(t *testing.T) {
	m, _ := newTableStack(t, "a", "b")
	m.SaveForm(context.Background(), "t.yaml", "r1.json", map[string]any{
		"tbl": []any{[]any{"a1", "b1"}},
	})

	// Column "x" inserted between a and b: new order [a, x, b] -> [0, -1, 1].
	n, err := m.RemapTableColumns("t.yaml", "tbl", []int{0, -1, 1})
	if err != nil || n != 1 {
		t.Fatalf("remap: n=%d err=%v", n, err)
	}
	row := loadRows(t, m, "r1.json")[0].([]any)
	if len(row) != 3 || row[0] != "a1" || row[1] != "" || row[2] != "b1" {
		t.Errorf("row = %v, want [a1 \"\" b1]", row)
	}
}

func TestRemapTableColumns_PreservesUpdatedStamp(t *testing.T) {
	m, _ := newTableStack(t, "a", "b")
	m.SaveForm(context.Background(), "t.yaml", "r1.json", map[string]any{
		"tbl": []any{[]any{"a1", "b1"}},
	})
	before := m.LoadForm("t.yaml", "r1.json").Meta.Updated

	if _, err := m.RemapTableColumns("t.yaml", "tbl", []int{1, 0}); err != nil {
		t.Fatalf("remap: %v", err)
	}
	after := m.LoadForm("t.yaml", "r1.json").Meta.Updated
	if before.At != after.At || before.Name != after.Name {
		t.Errorf("Updated changed: before=%+v after=%+v (structural remap must not re-stamp)", before, after)
	}
}

func TestRemapTableColumns_LoopNestedTable(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	col := func(keys ...string) []any {
		out := make([]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, map[string]any{"value": k, "type": "string", "label": k})
		}
		return out
	}
	// A loop "rows" whose child is a table "tbl" with columns a,b,c.
	if err := tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{
			{Key: "rows", Type: "loopstart"},
			{Key: "tbl", Type: "table", Options: col("a", "b", "c")},
			{Key: "rows", Type: "loopstop"},
		},
	}); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	r := m.SaveForm(context.Background(), "t.yaml", "r1.json", map[string]any{
		"rows": []any{
			map[string]any{"tbl": []any{[]any{"a1", "b1", "c1"}}},
			map[string]any{"tbl": []any{[]any{"a2", "b2", "c2"}}},
		},
	})
	if !r.Success {
		t.Fatalf("seed record: %+v", r)
	}

	// Reorder columns to [a, c, b] inside the loop-nested table.
	n, err := m.RemapTableColumns("t.yaml", "tbl", []int{0, 2, 1})
	if err != nil || n != 1 {
		t.Fatalf("remap: n=%d err=%v", n, err)
	}
	loop := m.LoadForm("t.yaml", "r1.json").Data["rows"].([]any)
	r0 := loop[0].(map[string]any)["tbl"].([]any)[0].([]any)
	r1 := loop[1].(map[string]any)["tbl"].([]any)[0].([]any)
	if r0[0] != "a1" || r0[1] != "c1" || r0[2] != "b1" {
		t.Errorf("loop row0 table = %v, want [a1 c1 b1]", r0)
	}
	if r1[0] != "a2" || r1[1] != "c2" || r1[2] != "b2" {
		t.Errorf("loop row1 table = %v, want [a2 c2 b2]", r1)
	}
}

func TestRemapTableColumns_SkipsRecordsWithoutField(t *testing.T) {
	m, _ := newTableStack(t, "a", "b")
	m.SaveForm(context.Background(), "t.yaml", "empty.json", map[string]any{})

	n, err := m.RemapTableColumns("t.yaml", "tbl", []int{1, 0})
	if err != nil || n != 0 {
		t.Fatalf("remap: n=%d err=%v, want 0 rewritten", n, err)
	}
}
