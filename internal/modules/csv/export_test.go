package csv

import (
	"errors"
	"reflect"
	"testing"
)

func TestBuildExportRows_NoAlignment_PlainColumns(t *testing.T) {
	fields := []FieldSpec{
		{Key: "name", Type: "text"},
		{Key: "city", Type: "text"},
	}
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "Name", SourceKeys: []string{"name"}},
			{Header: "City", SourceKeys: []string{"city"}},
		},
	}
	entries := []map[string]any{
		{"name": "Alice", "city": "Amsterdam"},
		{"name": "Bob", "city": "Berlin"},
	}
	got := BuildExportRows(plan, entries, fields)
	want := [][]string{
		{"Name", "City"},
		{"Alice", "Amsterdam"},
		{"Bob", "Berlin"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestBuildExportRows_ComputedConcatColumn(t *testing.T) {
	fields := []FieldSpec{
		{Key: "first", Type: "text"},
		{Key: "last", Type: "text"},
	}
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "Full", SourceKeys: []string{"first", "last"}, Separator: " "},
		},
	}
	entries := []map[string]any{
		{"first": "Alice", "last": "Adams"},
	}
	got := BuildExportRows(plan, entries, fields)
	want := [][]string{
		{"Full"},
		{"Alice Adams"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestBuildExportRows_TransformAppliedPerColumn(t *testing.T) {
	fields := []FieldSpec{{Key: "name", Type: "text"}}
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "Name", SourceKeys: []string{"name"}, Transform: Transform{Rule: "uppercase"}},
		},
	}
	entries := []map[string]any{{"name": "alice"}}
	got := BuildExportRows(plan, entries, fields)
	if got[1][0] != "ALICE" {
		t.Errorf("transform not applied: %v", got)
	}
}

func TestBuildExportRows_BooleanFormatted(t *testing.T) {
	fields := []FieldSpec{{Key: "active", Type: "boolean"}}
	plan := ExportPlan{
		Columns: []ExportColumn{{Header: "Active", SourceKeys: []string{"active"}}},
	}
	entries := []map[string]any{
		{"active": true},
		{"active": false},
		{"active": nil}, // missing
	}
	got := BuildExportRows(plan, entries, fields)
	if got[1][0] != "true" || got[2][0] != "false" || got[3][0] != "" {
		t.Errorf("boolean formatting: %v", got)
	}
}

func TestBuildExportRows_ListAligned_OneRowPerItem(t *testing.T) {
	fields := []FieldSpec{
		{Key: "name", Type: "text"},
		{Key: "phones", Type: "list"},
	}
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "Name", SourceKeys: []string{"name"}},
			{Header: "Phone", SourceKeys: []string{"phones"}},
		},
		AlignSource: "phones",
	}
	entries := []map[string]any{
		{"name": "Alice", "phones": []any{"555-1", "555-2"}},
		{"name": "Bob", "phones": []any{}}, // empty → still emits one row
	}
	got := BuildExportRows(plan, entries, fields)
	want := [][]string{
		{"Name", "Phone"},
		{"Alice", "555-1"},
		{"Alice", "555-2"},
		{"Bob", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("aligned list rows = %v, want %v", got, want)
	}
}

func TestBuildExportRows_TableAligned_DottedSubkey(t *testing.T) {
	// Table field "owners" with columns first / last, stored as
	// positional arrays. Aligned export pulls one CSV row per owner,
	// resolving "owners.first" / "owners.last" via the option list.
	fields := []FieldSpec{
		{Key: "company", Type: "text"},
		{Key: "owners", Type: "table", Options: []any{
			map[string]any{"value": "first", "label": "First"},
			map[string]any{"value": "last", "label": "Last"},
		}},
	}
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "Company", SourceKeys: []string{"company"}},
			{Header: "First", SourceKeys: []string{"owners.first"}},
			{Header: "Last", SourceKeys: []string{"owners.last"}},
		},
		AlignSource: "owners",
	}
	entries := []map[string]any{
		{"company": "Beatles", "owners": []any{
			[]any{"John", "Lennon"},
			[]any{"Paul", "McCartney"},
		}},
	}
	got := BuildExportRows(plan, entries, fields)
	want := [][]string{
		{"Company", "First", "Last"},
		{"Beatles", "John", "Lennon"},
		{"Beatles", "Paul", "McCartney"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("table-aligned rows = %v, want %v", got, want)
	}
}

func TestBuildExportRows_AlignedRootKeyWithoutSubkey(t *testing.T) {
	// Bare alignment root for a list field returns the item's string form.
	fields := []FieldSpec{{Key: "tags", Type: "list"}}
	plan := ExportPlan{
		Columns:     []ExportColumn{{Header: "Tag", SourceKeys: []string{"tags"}}},
		AlignSource: "tags",
	}
	entries := []map[string]any{
		{"tags": []any{"red", "green"}},
	}
	got := BuildExportRows(plan, entries, fields)
	if len(got) != 3 || got[1][0] != "red" || got[2][0] != "green" {
		t.Errorf("bare aligned root: %v", got)
	}
}

func TestBuildExportRows_UnknownAlignmentDisabled(t *testing.T) {
	// AlignSource points to a non-list/table field → behave as if not set.
	fields := []FieldSpec{{Key: "name", Type: "text"}}
	plan := ExportPlan{
		Columns:     []ExportColumn{{Header: "Name", SourceKeys: []string{"name"}}},
		AlignSource: "name",
	}
	entries := []map[string]any{{"name": "Alice"}}
	got := BuildExportRows(plan, entries, fields)
	if len(got) != 2 || got[1][0] != "Alice" {
		t.Errorf("bad alignment should no-op: %v", got)
	}
}

func TestBuildExportRows_AlignedSubkeyOnNonAlignedColumn(t *testing.T) {
	// "Aligned" column resolution only triggers when the source root
	// matches AlignSource. Other columns keep their entry-level value
	// across the unrolled rows.
	fields := []FieldSpec{
		{Key: "name", Type: "text"},
		{Key: "phones", Type: "list"},
	}
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "Name", SourceKeys: []string{"name"}},
			{Header: "Phone", SourceKeys: []string{"phones"}},
		},
		AlignSource: "phones",
	}
	entries := []map[string]any{
		{"name": "Alice", "phones": []any{"a", "b"}},
	}
	got := BuildExportRows(plan, entries, fields)
	if got[1][0] != "Alice" || got[2][0] != "Alice" {
		t.Errorf("non-aligned col should repeat: %v", got)
	}
}

func TestBuildExportRows_MissingFieldInPlanIsBlank(t *testing.T) {
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "Ghost", SourceKeys: []string{"nonexistent"}},
		},
	}
	entries := []map[string]any{{"name": "Alice"}}
	got := BuildExportRows(plan, entries, nil)
	if got[1][0] != "" {
		t.Errorf("missing field should be blank, got %q", got[1][0])
	}
}

func TestBuildExportRows_NoSourceKeysIsBlank(t *testing.T) {
	plan := ExportPlan{
		Columns: []ExportColumn{{Header: "Empty", SourceKeys: nil}},
	}
	entries := []map[string]any{{"name": "Alice"}}
	got := BuildExportRows(plan, entries, []FieldSpec{{Key: "name", Type: "text"}})
	if got[1][0] != "" {
		t.Errorf("empty sourceKeys should be blank: %v", got)
	}
}

func TestBuildExportRows_HeaderOnlyWhenNoEntries(t *testing.T) {
	plan := ExportPlan{
		Columns: []ExportColumn{{Header: "Name", SourceKeys: []string{"name"}}},
	}
	got := BuildExportRows(plan, nil, []FieldSpec{{Key: "name", Type: "text"}})
	if len(got) != 1 || got[0][0] != "Name" {
		t.Errorf("expected header-only, got %v", got)
	}
}

func TestBuildExportRows_TableSubkeyOptionAsBareString(t *testing.T) {
	// Some templates declare table column options as bare strings
	// instead of {value,label}. The bare value doubles as the lookup key.
	fields := []FieldSpec{
		{Key: "tbl", Type: "table", Options: []any{"first", "last"}},
	}
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "F", SourceKeys: []string{"tbl.first"}},
			{Header: "L", SourceKeys: []string{"tbl.last"}},
		},
		AlignSource: "tbl",
	}
	entries := []map[string]any{
		{"tbl": []any{[]any{"John", "Doe"}}},
	}
	got := BuildExportRows(plan, entries, fields)
	if got[1][0] != "John" || got[1][1] != "Doe" {
		t.Errorf("bare-string options not resolved: %v", got)
	}
}

func TestBuildExportRows_AlignedObjectShapedTableItem(t *testing.T) {
	// Legacy/foreign data: table items are objects keyed by column name
	// rather than positional arrays. The dotted subkey indexes by key.
	fields := []FieldSpec{
		{Key: "owners", Type: "table", Options: []any{
			map[string]any{"value": "first", "label": "First"},
			map[string]any{"value": "last", "label": "Last"},
		}},
	}
	plan := ExportPlan{
		Columns: []ExportColumn{
			{Header: "First", SourceKeys: []string{"owners.first"}},
			{Header: "Last", SourceKeys: []string{"owners.last"}},
		},
		AlignSource: "owners",
	}
	entries := []map[string]any{
		{"owners": []any{
			map[string]any{"first": "John", "last": "Lennon"},
		}},
	}
	got := BuildExportRows(plan, entries, fields)
	if got[1][0] != "John" || got[1][1] != "Lennon" {
		t.Errorf("object-shaped table item not resolved by key: %v", got)
	}
}

func TestBuildExportRows_AlignedUnknownSubkeyIsBlank(t *testing.T) {
	// Subkey not present in the field option list -> findOptionIndex -1 -> blank.
	fields := []FieldSpec{
		{Key: "owners", Type: "table", Options: []any{
			map[string]any{"value": "first", "label": "First"},
		}},
	}
	plan := ExportPlan{
		Columns:     []ExportColumn{{Header: "Ghost", SourceKeys: []string{"owners.ghost"}}},
		AlignSource: "owners",
	}
	entries := []map[string]any{
		{"owners": []any{[]any{"John"}}},
	}
	got := BuildExportRows(plan, entries, fields)
	if got[1][0] != "" {
		t.Errorf("unknown subkey should be blank, got %q", got[1][0])
	}
}

func TestBuildExportRows_AlignedBareRootMapItemAsJSON(t *testing.T) {
	// Bare alignment root (no subkey) over a map-shaped item -> JSON string.
	fields := []FieldSpec{{Key: "rows", Type: "list"}}
	plan := ExportPlan{
		Columns:     []ExportColumn{{Header: "Row", SourceKeys: []string{"rows"}}},
		AlignSource: "rows",
	}
	entries := []map[string]any{
		{"rows": []any{map[string]any{"k": "v"}}},
	}
	got := BuildExportRows(plan, entries, fields)
	if got[1][0] != `{"k":"v"}` {
		t.Errorf("bare map item should be JSON, got %q", got[1][0])
	}
}

func TestBuildExportRows_AlignedNilItemIsBlank(t *testing.T) {
	// A nil item in the aligned array yields a blank cell.
	fields := []FieldSpec{{Key: "tags", Type: "list"}}
	plan := ExportPlan{
		Columns:     []ExportColumn{{Header: "Tag", SourceKeys: []string{"tags"}}},
		AlignSource: "tags",
	}
	entries := []map[string]any{
		{"tags": []any{nil, "red"}},
	}
	got := BuildExportRows(plan, entries, fields)
	if got[1][0] != "" || got[2][0] != "red" {
		t.Errorf("nil aligned item should be blank: %v", got)
	}
}

func TestBuildExportRows_AlignedEmptyArrayHitsIndexGuard(t *testing.T) {
	// Empty aligned array: the loop still runs once at index 0, so the
	// alignIdx >= len(arr) guard fires and the aligned cell is blank.
	fields := []FieldSpec{{Key: "long", Type: "list"}}
	plan := ExportPlan{
		Columns:     []ExportColumn{{Header: "Long", SourceKeys: []string{"long"}}},
		AlignSource: "long",
	}
	entries := []map[string]any{
		{"long": []any{}},
	}
	got := BuildExportRows(plan, entries, fields)
	if len(got) != 2 || got[1][0] != "" {
		t.Errorf("empty aligned root should emit one blank row: %v", got)
	}
}

// stubForms is a controllable formsSource: ListForms can fail, and
// LoadFormData returns whatever data map is keyed by datafile (nil for a
// miss, exercising the skip/blank branches).
type stubForms struct {
	files   []string
	listErr error
	data    map[string]map[string]any
}

func (s stubForms) ListForms(string) ([]string, error) { return s.files, s.listErr }
func (s stubForms) LoadFormData(_, datafile string) map[string]any {
	if s.data == nil {
		return nil
	}
	return s.data[datafile]
}

func TestExport_NoFormsDependency(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetTemplate(fakeTemplateSource{fields: []FieldSpec{{Key: "name", Type: "text"}}})
	got := m.Export("t.yaml", ExportPlan{})
	if got.Error == "" {
		t.Error("Export without forms dep should set Error")
	}
}

func TestExport_TemplateDepError(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{})
	m.SetTemplate(fakeTemplateSource{err: errors.New("no schema")})
	got := m.Export("t.yaml", ExportPlan{})
	if got.Error != "no schema" {
		t.Errorf("Export Error = %q, want propagated template error", got.Error)
	}
}

func TestExport_NilTemplateDependency(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{})
	// No SetTemplate: fieldsFor must report the missing dependency.
	got := m.Export("t.yaml", ExportPlan{})
	if got.Error == "" {
		t.Error("Export without template dep should set Error")
	}
}

func TestExport_ListFormsError(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{listErr: errors.New("list boom")})
	m.SetTemplate(fakeTemplateSource{fields: []FieldSpec{{Key: "name", Type: "text"}}})
	got := m.Export("t.yaml", ExportPlan{})
	if got.Error != "list boom" {
		t.Errorf("Export Error = %q, want list boom", got.Error)
	}
}

func TestExport_SkipsNilFormData(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{
		files: []string{"a.meta.json", "b.meta.json"},
		data:  map[string]map[string]any{"a.meta.json": {"name": "Alice"}}, // b is a miss -> nil
	})
	m.SetTemplate(fakeTemplateSource{fields: []FieldSpec{{Key: "name", Type: "text"}}})
	plan := ExportPlan{Columns: []ExportColumn{{Header: "Name", SourceKeys: []string{"name"}}}}
	got := m.Export("t.yaml", plan)
	if got.Error != "" {
		t.Fatalf("unexpected error: %q", got.Error)
	}
	if got.Count != 1 {
		t.Errorf("Count = %d, want 1 (nil form data skipped)", got.Count)
	}
	if len(got.Rows) != 2 { // header + one data row
		t.Errorf("Rows = %v, want header + 1", got.Rows)
	}
}

func TestPreviewExport_NoFormsDependency(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetTemplate(fakeTemplateSource{fields: []FieldSpec{{Key: "name", Type: "text"}}})
	got := m.PreviewExport("t.yaml", ExportPlan{})
	if got.Error == "" {
		t.Error("PreviewExport without forms dep should set Error")
	}
}

func TestPreviewExport_TemplateDepError(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{})
	m.SetTemplate(fakeTemplateSource{err: errors.New("no schema")})
	got := m.PreviewExport("t.yaml", ExportPlan{})
	if got.Error != "no schema" {
		t.Errorf("PreviewExport Error = %q, want propagated", got.Error)
	}
}

func TestPreviewExport_ListFormsError(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{listErr: errors.New("list boom")})
	m.SetTemplate(fakeTemplateSource{fields: []FieldSpec{{Key: "name", Type: "text"}}})
	got := m.PreviewExport("t.yaml", ExportPlan{})
	if got.Error != "list boom" {
		t.Errorf("PreviewExport Error = %q, want list boom", got.Error)
	}
}

func TestPreviewExport_NoEntriesReturnsBlankCells(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{files: nil})
	m.SetTemplate(fakeTemplateSource{fields: []FieldSpec{{Key: "name", Type: "text"}}})
	plan := ExportPlan{Columns: []ExportColumn{{Header: "A"}, {Header: "B"}}}
	got := m.PreviewExport("t.yaml", plan)
	if got.Error != "" {
		t.Fatalf("unexpected error: %q", got.Error)
	}
	if len(got.Cells) != 2 {
		t.Errorf("Cells = %v, want 2 blank cells", got.Cells)
	}
	for i, c := range got.Cells {
		if c != "" {
			t.Errorf("Cells[%d] = %q, want blank", i, c)
		}
	}
}

func TestPreviewExport_NilFirstFormReturnsBlankCells(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{files: []string{"a.meta.json"}, data: nil}) // LoadFormData -> nil
	m.SetTemplate(fakeTemplateSource{fields: []FieldSpec{{Key: "name", Type: "text"}}})
	plan := ExportPlan{Columns: []ExportColumn{{Header: "A"}}}
	got := m.PreviewExport("t.yaml", plan)
	if got.Error != "" || len(got.Cells) != 1 || got.Cells[0] != "" {
		t.Errorf("expected one blank cell, got %+v", got)
	}
}

func TestPreviewExport_ReturnsSampleRow(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetForms(stubForms{
		files: []string{"a.meta.json"},
		data:  map[string]map[string]any{"a.meta.json": {"name": "Alice"}},
	})
	m.SetTemplate(fakeTemplateSource{fields: []FieldSpec{{Key: "name", Type: "text"}}})
	plan := ExportPlan{Columns: []ExportColumn{{Header: "Name", SourceKeys: []string{"name"}}}}
	got := m.PreviewExport("t.yaml", plan)
	if got.Error != "" {
		t.Fatalf("unexpected error: %q", got.Error)
	}
	if len(got.Cells) != 1 || got.Cells[0] != "Alice" {
		t.Errorf("Cells = %v, want [Alice]", got.Cells)
	}
}
