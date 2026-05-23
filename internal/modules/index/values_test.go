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

func TestPickValues_NumberField(t *testing.T) {
	fields := []template.Field{{Key: "amount", Type: "number"}}
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

func TestPickValues_NumberFromString(t *testing.T) {
	fields := []template.Field{{Key: "amount", Type: "number"}}
	rows := pickValues(fields, map[string]any{"amount": "3.5"})
	got := findValue(rows, "amount", -1)
	if got == nil || got.Num == nil || *got.Num != 3.5 {
		t.Fatalf("num = %v, want 3.5", got)
	}
}

func TestPickValues_NonNumericNumberHasNilNum(t *testing.T) {
	fields := []template.Field{{Key: "amount", Type: "number"}}
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
	fields := []template.Field{{Key: "due", Type: "date"}}
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
	// epoch for 2026-05-23T00:00:00Z
	if got.Num == nil {
		t.Fatal("date row must carry epoch num for range/min/max")
	}
	const wantEpoch = 1779494400 // 2026-05-23 UTC
	if *got.Num != wantEpoch {
		t.Errorf("num = %v, want epoch %d", *got.Num, wantEpoch)
	}
}

func TestPickValues_UnparseableDateSkipped(t *testing.T) {
	fields := []template.Field{{Key: "due", Type: "date"}}
	rows := pickValues(fields, map[string]any{"due": "someday"})
	if findValue(rows, "due", -1) != nil {
		t.Error("unparseable date should not produce a row")
	}
}

func TestPickValues_BooleanField(t *testing.T) {
	fields := []template.Field{{Key: "done", Type: "boolean"}}
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
	fields := []template.Field{{Key: "status", Type: "dropdown"}}
	rows := pickValues(fields, map[string]any{"status": "open"})
	got := findValue(rows, "status", -1)
	if got == nil || got.ValueType != "text" || got.Text != "open" {
		t.Fatalf("dropdown row = %v, want text/open", got)
	}
}

func TestPickValues_MultioptionFanOut(t *testing.T) {
	fields := []template.Field{{Key: "labels", Type: "multioption"}}
	rows := pickValues(fields, map[string]any{"labels": []any{"a", "b"}})
	if findValue(rows, "labels", -1) == nil {
		t.Fatal("expected at least one labels row")
	}
	count := 0
	for _, r := range rows {
		if r.FieldKey == "labels" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("multioption produced %d rows, want 2 (one per selected)", count)
	}
}

func TestPickValues_TableMixedColumns(t *testing.T) {
	fields := []template.Field{{
		Key:  "items",
		Type: "table",
		Options: []any{
			map[string]any{"value": "name", "type": "string", "label": "Name"},
			map[string]any{"value": "qty", "type": "number", "label": "Qty"},
			map[string]any{"value": "when", "type": "date", "label": "When"},
		},
	}}
	data := map[string]any{"items": []any{
		[]any{"apple", float64(3), "2026-01-02"},
		[]any{"pear", "5", "2026-01-03"},
	}}

	rows := pickValues(fields, data)

	// Column 1 (qty) must be two numeric rows summing to 8.
	sum := 0.0
	qtyRows := 0
	for _, r := range rows {
		if r.FieldKey == "items" && r.Col != nil && *r.Col == 1 {
			qtyRows++
			if r.ValueType != "number" {
				t.Errorf("qty col value_type = %q, want number", r.ValueType)
			}
			if r.Num != nil {
				sum += *r.Num
			}
		}
	}
	if qtyRows != 2 {
		t.Fatalf("qty rows = %d, want 2", qtyRows)
	}
	if sum != 8 {
		t.Errorf("qty sum = %v, want 8", sum)
	}

	// Column 2 (when) must be date-typed with ISO text.
	whenRow := findValue(rows, "items", 2)
	if whenRow == nil || whenRow.ValueType != "date" {
		t.Fatalf("when col row = %v, want date", whenRow)
	}
}

func TestPickValues_SkipsFreeTextAndTags(t *testing.T) {
	fields := []template.Field{
		{Key: "notes", Type: "textarea"},
		{Key: "title", Type: "text"},
		{Key: "tags", Type: "tags"},
		{Key: "gid", Type: "guid"},
	}
	data := map[string]any{
		"notes": "long body", "title": "hello",
		"tags": []any{"x"}, "gid": "abc",
	}
	rows := pickValues(fields, data)
	if len(rows) != 0 {
		t.Errorf("expected no value rows for free-text/tags/guid, got %d: %+v", len(rows), rows)
	}
}
