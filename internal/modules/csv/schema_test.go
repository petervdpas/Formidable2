package csv

import (
	"reflect"
	"slices"
	"testing"
)

func gapFields() []FieldSpec {
	return []FieldSpec{
		{Key: "id", Type: "text", Label: "ID"},
		{Key: "naam", Type: "text", Label: "Naam"},
		{Key: "code", Type: "code", Label: "Code"}, // excluded type
		{Key: "tags", Type: "list", Label: "Tags"},
		{Key: "gap", Type: "table", Label: "GAP Analyse", Options: []any{
			map[string]any{"value": "onderdeel", "label": "Onderdeel"},
			map[string]any{"value": "gap", "label": "GAP"},
			map[string]any{"value": "actie", "label": "Actie"},
		}},
	}
}

type fakeTemplateSource struct {
	fields []FieldSpec
	err    error
}

func (f fakeTemplateSource) Fields(string) ([]FieldSpec, error) { return f.fields, f.err }

func TestMappableFieldsForTemplate_StripsExcludedTypes(t *testing.T) {
	m := &Manager{tpl: fakeTemplateSource{fields: gapFields()}}
	got, err := m.MappableFieldsForTemplate("t.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, f := range got {
		keys = append(keys, f.Key)
	}
	want := []string{"id", "naam", "tags", "gap"} // code (excluded) dropped
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("mappable = %v, want %v", keys, want)
	}
}

func TestMappableFieldsForTemplate_NoTemplateDep(t *testing.T) {
	m := &Manager{}
	if _, err := m.MappableFieldsForTemplate("t.yaml"); err == nil {
		t.Fatal("expected error when template dependency not configured")
	}
}

func TestDefaultExportPlan_NoAlignment_OneColumnPerMappableField(t *testing.T) {
	plan := DefaultExportPlan(gapFields(), "")
	var keys []string
	for _, c := range plan.Columns {
		keys = append(keys, c.SourceKeys[0])
	}
	want := []string{"id", "naam", "tags", "gap"} // code excluded
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("columns = %v, want %v", keys, want)
	}
	if plan.AlignSource != "" {
		t.Errorf("AlignSource = %q, want empty", plan.AlignSource)
	}
}

func TestDefaultExportPlan_TableAligned_ExpandsIntoSubColumns(t *testing.T) {
	plan := DefaultExportPlan(gapFields(), "gap")
	var keys, headers []string
	for _, c := range plan.Columns {
		keys = append(keys, c.SourceKeys[0])
		headers = append(headers, c.Header)
	}
	want := []string{"id", "naam", "tags", "gap.onderdeel", "gap.gap", "gap.actie"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("aligned columns = %v, want %v", keys, want)
	}
	for _, h := range []string{"gap-onderdeel", "gap-gap", "gap-actie"} {
		if !slices.Contains(headers, h) {
			t.Errorf("expected header %q in %v", h, headers)
		}
	}
	if plan.AlignSource != "gap" {
		t.Errorf("AlignSource = %q, want gap", plan.AlignSource)
	}
}

func TestDefaultExportPlan_ListAligned_KeepsSingleColumn(t *testing.T) {
	plan := DefaultExportPlan(gapFields(), "tags")
	for _, c := range plan.Columns {
		if c.SourceKeys[0] == "tags" && len(c.SourceKeys) != 1 {
			t.Fatalf("list align should stay one bare column, got %v", c.SourceKeys)
		}
		if c.SourceKeys[0] == "tags.x" {
			t.Fatalf("list must not expand into subkeys: %v", c.SourceKeys)
		}
	}
	if plan.AlignSource != "tags" {
		t.Errorf("AlignSource = %q, want tags", plan.AlignSource)
	}
}

func TestDefaultExportPlan_UnknownAlignSourceIgnored(t *testing.T) {
	plan := DefaultExportPlan(gapFields(), "nope")
	if plan.AlignSource != "" {
		t.Errorf("AlignSource = %q, want empty for unknown field", plan.AlignSource)
	}
}

func TestDefaultExportPlan_NonAlignableTypeNotAccepted(t *testing.T) {
	// A scalar field is not a valid alignment target.
	plan := DefaultExportPlan(gapFields(), "naam")
	if plan.AlignSource != "" {
		t.Errorf("AlignSource = %q, want empty for non-list/table field", plan.AlignSource)
	}
}

func TestAlignableFields_OnlyListAndTable(t *testing.T) {
	got := AlignableFields(gapFields())
	var vals []string
	for _, o := range got {
		vals = append(vals, o.Value)
	}
	want := []string{"tags", "gap"}
	if !reflect.DeepEqual(vals, want) {
		t.Fatalf("alignable = %v, want %v", vals, want)
	}
}

func TestSourceOptions_AddsTableSubkeysWhenAligned(t *testing.T) {
	got := SourceOptions(gapFields(), "gap")
	var vals []string
	for _, o := range got {
		vals = append(vals, o.Value)
	}
	for _, want := range []string{"id", "naam", "tags", "gap", "gap.onderdeel", "gap.gap", "gap.actie"} {
		if !slices.Contains(vals, want) {
			t.Errorf("expected source %q in %v", want, vals)
		}
	}
}

func TestSourceOptions_NoSubkeysWithoutTableAlignment(t *testing.T) {
	got := SourceOptions(gapFields(), "")
	for _, o := range got {
		if o.Value == "gap.onderdeel" {
			t.Fatalf("subkeys must not appear without table alignment: %v", got)
		}
	}
}

func TestTableSubkeys_LabelsUseColumnLabels(t *testing.T) {
	subs := tableSubkeys(gapFields()[4]) // gap
	if len(subs) != 3 {
		t.Fatalf("want 3 subkeys, got %d", len(subs))
	}
	if subs[0].Label != "GAP Analyse → Onderdeel" {
		t.Errorf("label = %q", subs[0].Label)
	}
}
