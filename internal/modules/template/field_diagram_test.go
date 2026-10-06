package template

import (
	"reflect"
	"strings"
	"testing"
)

func lineageTable(key string) Field {
	return Field{Key: key, Type: "table", Format: TablePatternDataLineage, Options: ApplyTablePattern(nil, TablePatternDataLineage)}
}

func diagramField(source string) Field {
	return Field{Key: "lineage", Type: "diagram", Label: "Lineage", Options: []any{
		map[string]any{"value": DiagramSource, "label": source},
	}}
}

func diagramErrs(errs []ValidationError) []string {
	var out []string
	for _, e := range errs {
		if strings.HasPrefix(e.Type, "diagram-") {
			out = append(out, e.Type)
		}
	}
	return out
}

func TestDiagramField_IsVirtualAndKnown(t *testing.T) {
	if !IsKnownFieldType("diagram") || !IsVirtualFieldType("diagram") {
		t.Fatal("diagram must be a known virtual field type")
	}
}

func TestDiagramField_OnlyOptionRowIsSource(t *testing.T) {
	rows := fieldDescriptors["diagram"].OptionsShape.Rows
	if len(rows) != 1 || rows[0].Defaults["value"] != DiagramSource || rows[0].Input != "table-field" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestDiagramField_BoundToLineageTableIsValid(t *testing.T) {
	tpl := &Template{Fields: []Field{lineageTable("mapping"), diagramField("mapping")}}
	if got := diagramErrs(Validate(tpl)); len(got) != 0 {
		t.Fatalf("unexpected %v", got)
	}
}

func TestDiagramField_Errors(t *testing.T) {
	regular := Field{Key: "plain", Type: "table", Options: ApplyTablePattern(nil, TablePatternDataLineage)}
	cases := []struct {
		name   string
		fields []Field
		want   string
	}{
		{"no pattern table on template", []Field{regular, diagramField("plain")}, "diagram-field-no-pattern-table"},
		{"no table at all", []Field{diagramField("")}, "diagram-field-no-pattern-table"},
		{"pattern table only inside a loop", []Field{
			{Key: "L", Type: "loopstart"}, lineageTable("mapping"), {Key: "L", Type: "loopstop"},
			diagramField("mapping"),
		}, "diagram-field-no-pattern-table"},
		{"missing source", []Field{lineageTable("mapping"), diagramField("")}, "diagram-field-missing-source"},
		{"unknown source", []Field{lineageTable("mapping"), diagramField("ghost")}, "diagram-field-unknown-source"},
		{"source not a table", []Field{lineageTable("mapping"), {Key: "t", Type: "text"}, diagramField("t")}, "diagram-field-source-not-table"},
		{"source table without pattern", []Field{lineageTable("mapping"), regular, diagramField("plain")}, "diagram-field-source-no-pattern"},
		{"source table inside a loop", []Field{
			lineageTable("mapping"),
			{Key: "L", Type: "loopstart"}, lineageTable("inner"), {Key: "L", Type: "loopstop"},
			diagramField("inner"),
		}, "diagram-field-source-not-root"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := diagramErrs(Validate(&Template{Fields: c.fields})); !reflect.DeepEqual(got, []string{c.want}) {
				t.Fatalf("got %v, want [%s]", got, c.want)
			}
		})
	}
}

func TestDiagramBindingOf_ColumnsComeFromThePattern(t *testing.T) {
	tpl := &Template{Fields: []Field{lineageTable("mapping"), diagramField("mapping")}}
	b, ok := DiagramBindingOf(tpl, "lineage")
	want := DiagramBinding{
		Projection: DiagramProjectionLineage,
		Source:     "mapping",
		Columns:    map[string]int{DiagramFromEntity: 0, DiagramFromAttr: 1, DiagramToEntity: 2, DiagramToAttr: 3, DiagramLabel: 5, DiagramExample: 4},
	}
	if !ok || !reflect.DeepEqual(b, want) {
		t.Fatalf("binding = %+v ok=%v, want %+v", b, ok, want)
	}
}

func TestDiagramBindingOf_FollowsReorderedPatternColumns(t *testing.T) {
	table := lineageTable("mapping")
	table.Options = append([]any{map[string]any{"value": "notes", "label": "N"}}, table.Options...)
	b, ok := DiagramBindingOf(&Template{Fields: []Field{table, diagramField("mapping")}}, "lineage")
	if !ok || b.Columns[DiagramFromEntity] != 1 || b.Columns[DiagramLabel] != 6 {
		t.Fatalf("binding = %+v ok=%v", b, ok)
	}
}

func TestDiagramBindingOf_UnknownOrInvalidIsNotOK(t *testing.T) {
	tpl := &Template{Fields: []Field{lineageTable("mapping"), diagramField("ghost"), {Key: "x", Type: "text"}}}
	for _, key := range []string{"lineage", "x", "nope", ""} {
		if _, ok := DiagramBindingOf(tpl, key); ok {
			t.Fatalf("DiagramBindingOf(%q) ok, want false", key)
		}
	}
	if _, ok := DiagramBindingOf(nil, "lineage"); ok {
		t.Fatal("nil template must not resolve")
	}
}

func TestDiagramTableKeys_OnlyRootPatternTables(t *testing.T) {
	fields := []Field{
		lineageTable("a"),
		{Key: "plain", Type: "table"},
		{Key: "L", Type: "loopstart"}, lineageTable("inner"), {Key: "L", Type: "loopstop"},
		lineageTable("b"),
	}
	if got := DiagramTableKeys(fields); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
}
