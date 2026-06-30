package index

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// findValue returns the first row matching fieldKey and col (col=-1
// matches a scalar row, i.e. Col == nil), or nil.
func findValue(rows []FormValueRow, fieldKey string, col int) *FormValueRow {
	for i := range rows {
		r := &rows[i]
		if r.FieldKey != fieldKey {
			continue
		}
		if col < 0 {
			if r.Col == nil {
				return r
			}
			continue
		}
		if r.Col != nil && *r.Col == col {
			return r
		}
	}
	return nil
}

func countFor(rows []FormValueRow, fieldKey string) int {
	n := 0
	for _, r := range rows {
		if r.FieldKey == fieldKey {
			n++
		}
	}
	return n
}

func TestPickValues_NumberField(t *testing.T) {
	fields := []template.Field{{Key: "amount", Type: "number", UseInStatistics: true}}
	data := map[string]any{"amount": float64(42)}

	rows := pickValues(fields, data)
	got := findValue(rows, "amount", -1)
	if got == nil {
		t.Fatal("no row for amount")
	}
	if got.ValueType != "number" {
		t.Errorf("value_type = %q, want number", got.ValueType)
	}
	if got.Num == nil || *got.Num != 42 {
		t.Errorf("num = %v, want 42", got.Num)
	}
}

// A sequence field orders the collection, so its value must be materialised
// for the max-sibling and deck-sort queries even though the author never marks
// it use_in_statistics. This is the invariant that makes ordering queryable.
func TestPickValues_SequenceAlwaysIndexed(t *testing.T) {
	fields := []template.Field{{Key: "pos", Type: "sequence"}} // UseInStatistics deliberately false
	rows := pickValues(fields, map[string]any{"pos": float64(20)})
	got := findValue(rows, "pos", -1)
	if got == nil {
		t.Fatal("sequence value must be indexed even without use_in_statistics")
	}
	if got.ValueType != "number" {
		t.Errorf("value_type = %q, want number", got.ValueType)
	}
	if got.Num == nil || *got.Num != 20 {
		t.Errorf("num = %v, want 20", got.Num)
	}
}

func TestPickValues_NumberFromString(t *testing.T) {
	fields := []template.Field{{Key: "amount", Type: "number", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"amount": "3.5"})
	got := findValue(rows, "amount", -1)
	if got == nil || got.Num == nil || *got.Num != 3.5 {
		t.Fatalf("num = %v, want 3.5", got)
	}
}

func TestPickValues_NonNumericNumberHasNilNum(t *testing.T) {
	fields := []template.Field{{Key: "amount", Type: "number", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"amount": "n/a"})
	got := findValue(rows, "amount", -1)
	if got == nil {
		t.Fatal("expected a row even when unparseable")
	}
	if got.Num != nil {
		t.Errorf("num = %v, want nil for non-numeric", *got.Num)
	}
}

func TestPickValues_DateField(t *testing.T) {
	fields := []template.Field{{Key: "due", Type: "date", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"due": "2026-05-23"})
	got := findValue(rows, "due", -1)
	if got == nil {
		t.Fatal("no row for due")
	}
	if got.ValueType != "date" {
		t.Errorf("value_type = %q, want date", got.ValueType)
	}
	if got.Text != "2026-05-23" {
		t.Errorf("text = %q, want ISO 2026-05-23", got.Text)
	}
	if got.Num == nil {
		t.Fatal("date row must carry epoch num for range/min/max")
	}
	const wantEpoch = 1779494400 // 2026-05-23 UTC
	if *got.Num != wantEpoch {
		t.Errorf("num = %v, want epoch %d", *got.Num, wantEpoch)
	}
}

func TestPickValues_UnparseableDateSkipped(t *testing.T) {
	fields := []template.Field{{Key: "due", Type: "date", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"due": "someday"})
	if findValue(rows, "due", -1) != nil {
		t.Error("unparseable date should not produce a row")
	}
}

func TestPickValues_BooleanField(t *testing.T) {
	fields := []template.Field{{Key: "done", Type: "boolean", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"done": true})
	got := findValue(rows, "done", -1)
	if got == nil {
		t.Fatal("no row for done")
	}
	if got.ValueType != "bool" {
		t.Errorf("value_type = %q, want bool", got.ValueType)
	}
	if got.Text != "true" {
		t.Errorf("text = %q, want true", got.Text)
	}
	if got.Num == nil || *got.Num != 1 {
		t.Errorf("num = %v, want 1", got.Num)
	}
}

func TestPickValues_DropdownDistribution(t *testing.T) {
	fields := []template.Field{{Key: "status", Type: "dropdown", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"status": "open"})
	got := findValue(rows, "status", -1)
	if got == nil || got.ValueType != "text" || got.Text != "open" {
		t.Fatalf("dropdown row = %v, want text/open", got)
	}
}

func TestPickValues_MultioptionFanOut(t *testing.T) {
	fields := []template.Field{{Key: "labels", Type: "multioption", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"labels": []any{"a", "b"}})
	if findValue(rows, "labels", -1) == nil {
		t.Fatal("expected at least one labels row")
	}
	if got := countFor(rows, "labels"); got != 2 {
		t.Errorf("multioption produced %d rows, want 2 (one per selected)", got)
	}
}

// ── opt-in gate ──────────────────────────────────────────────────────

func TestPickValues_UnflaggedFieldIsSkipped(t *testing.T) {
	// Same number field, but use_in_statistics defaults false: no row.
	fields := []template.Field{{Key: "amount", Type: "number"}}
	rows := pickValues(fields, map[string]any{"amount": float64(42)})
	if len(rows) != 0 {
		t.Errorf("unflagged field produced %d rows, want 0: %+v", len(rows), rows)
	}
}

func TestPickValues_MixedFlaggedAndNot(t *testing.T) {
	fields := []template.Field{
		{Key: "amount", Type: "number", UseInStatistics: true},
		{Key: "ignored", Type: "number"}, // not flagged
	}
	rows := pickValues(fields, map[string]any{"amount": float64(1), "ignored": float64(2)})
	if findValue(rows, "amount", -1) == nil {
		t.Error("flagged field should be indexed")
	}
	if findValue(rows, "ignored", -1) != nil {
		t.Error("unflagged field must not be indexed")
	}
}

// ── list (single column, fans out one text row per item) ─────────────

func TestPickValues_ListFanOut(t *testing.T) {
	fields := []template.Field{{Key: "tagsish", Type: "list", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"tagsish": []any{"red", "blue", ""}})
	if got := countFor(rows, "tagsish"); got != 2 {
		t.Fatalf("list produced %d rows, want 2 (empty item skipped)", got)
	}
	r := findValue(rows, "tagsish", -1)
	if r == nil || r.ValueType != "text" {
		t.Fatalf("list row = %v, want text", r)
	}
}

func TestPickValues_ListUnflaggedSkipped(t *testing.T) {
	fields := []template.Field{{Key: "tagsish", Type: "list"}}
	rows := pickValues(fields, map[string]any{"tagsish": []any{"red"}})
	if len(rows) != 0 {
		t.Errorf("unflagged list produced %d rows, want 0", len(rows))
	}
}

func TestPickValues_ListNonArrayIsNoRows(t *testing.T) {
	fields := []template.Field{{Key: "tagsish", Type: "list", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"tagsish": "not-an-array"})
	if len(rows) != 0 {
		t.Errorf("non-array list produced %d rows, want 0", len(rows))
	}
}

// ── table (column subset selected by StatisticsColumns) ──────────────

func tableField(statCols ...string) template.Field {
	return template.Field{
		Key:               "items",
		Type:              "table",
		UseInStatistics:   true,
		StatisticsColumns: statCols,
		Options: []any{
			map[string]any{"value": "name", "type": "string", "label": "Name"},
			map[string]any{"value": "qty", "type": "number", "label": "Qty"},
			map[string]any{"value": "when", "type": "date", "label": "When"},
		},
	}
}

func tableData() map[string]any {
	return map[string]any{"items": []any{
		[]any{"apple", float64(3), "2026-01-02"},
		[]any{"pear", "5", "2026-01-03"},
	}}
}

func TestPickValues_TableOnlySelectedColumns(t *testing.T) {
	rows := pickValues([]template.Field{tableField("qty")}, tableData())

	// qty (col 1) indexed as two numeric rows summing to 8.
	sum := 0.0
	qtyRows := 0
	for _, r := range rows {
		if r.FieldKey == "items" && r.Col != nil && *r.Col == 1 {
			qtyRows++
			if r.ValueType != "number" {
				t.Errorf("qty value_type = %q, want number", r.ValueType)
			}
			if r.Num != nil {
				sum += *r.Num
			}
		}
	}
	if qtyRows != 2 || sum != 8 {
		t.Fatalf("qty rows=%d sum=%v, want 2 rows summing 8", qtyRows, sum)
	}
	// name (col 0) and when (col 2) were NOT selected: no rows.
	if findValue(rows, "items", 0) != nil {
		t.Error("unselected name column must not be indexed")
	}
	if findValue(rows, "items", 2) != nil {
		t.Error("unselected when column must not be indexed")
	}
}

func TestPickValues_TableMultipleSelectedColumns(t *testing.T) {
	rows := pickValues([]template.Field{tableField("qty", "when")}, tableData())
	if findValue(rows, "items", 1) == nil {
		t.Error("qty column should be indexed")
	}
	when := findValue(rows, "items", 2)
	if when == nil || when.ValueType != "date" {
		t.Fatalf("when col row = %v, want date", when)
	}
	if findValue(rows, "items", 0) != nil {
		t.Error("name column was not selected; must not be indexed")
	}
}

func TestPickValues_TableNoSelectedColumnsYieldsNothing(t *testing.T) {
	// Flagged table but empty StatisticsColumns: nothing to index.
	rows := pickValues([]template.Field{tableField()}, tableData())
	if countFor(rows, "items") != 0 {
		t.Errorf("table with no selected columns produced %d rows, want 0", countFor(rows, "items"))
	}
}

func TestPickValues_TableUnknownColumnIgnored(t *testing.T) {
	rows := pickValues([]template.Field{tableField("nope")}, tableData())
	if countFor(rows, "items") != 0 {
		t.Errorf("unknown selected column produced %d rows, want 0", countFor(rows, "items"))
	}
}

func TestPickValues_TableUnflaggedSkipped(t *testing.T) {
	f := tableField("qty")
	f.UseInStatistics = false
	rows := pickValues([]template.Field{f}, tableData())
	if countFor(rows, "items") != 0 {
		t.Errorf("unflagged table produced %d rows, want 0", countFor(rows, "items"))
	}
}

func TestPickValues_TableNonMatrixDataIsSafe(t *testing.T) {
	rows := pickValues([]template.Field{tableField("qty")}, map[string]any{"items": "garbage"})
	if countFor(rows, "items") != 0 {
		t.Errorf("non-matrix table data produced %d rows, want 0", countFor(rows, "items"))
	}
}

// ── free-text types never index, even when flagged ───────────────────

func TestPickValues_TextFieldIndexedWhenFlagged(t *testing.T) {
	fields := []template.Field{{Key: "base-table", Type: "text", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"base-table": "scm.Customer"})
	got := findValue(rows, "base-table", -1)
	if got == nil || got.ValueType != "text" || got.Text != "scm.Customer" {
		t.Fatalf("text row = %v, want text/scm.Customer", got)
	}
}

func TestPickValues_TextFieldUnflaggedSkipped(t *testing.T) {
	fields := []template.Field{{Key: "base-table", Type: "text"}}
	rows := pickValues(fields, map[string]any{"base-table": "scm.Customer"})
	if len(rows) != 0 {
		t.Errorf("unflagged text produced %d rows, want 0", len(rows))
	}
}

func TestPickValues_LongTextTypesNeverIndexEvenWhenFlagged(t *testing.T) {
	fields := []template.Field{
		{Key: "notes", Type: "textarea", UseInStatistics: true},
		{Key: "gid", Type: "guid", UseInStatistics: true},
	}
	data := map[string]any{"notes": "long body", "gid": "abc"}
	rows := pickValues(fields, data)
	if len(rows) != 0 {
		t.Errorf("expected no rows for textarea/guid even when flagged, got %d: %+v", len(rows), rows)
	}
}

func TestPickValues_TagsFanOut(t *testing.T) {
	fields := []template.Field{{Key: "tags", Type: "tags", UseInStatistics: true}}
	rows := pickValues(fields, map[string]any{"tags": []any{"go", "vue", ""}})
	if got := countFor(rows, "tags"); got != 2 {
		t.Fatalf("tags produced %d rows, want 2 (empty entry skipped)", got)
	}
	r := findValue(rows, "tags", -1)
	if r == nil || r.ValueType != "text" {
		t.Fatalf("tags row = %v, want text", r)
	}
}

func TestPickValues_TagsUnflaggedSkipped(t *testing.T) {
	fields := []template.Field{{Key: "tags", Type: "tags"}}
	rows := pickValues(fields, map[string]any{"tags": []any{"go"}})
	if len(rows) != 0 {
		t.Errorf("unflagged tags produced %d rows, want 0", len(rows))
	}
}
