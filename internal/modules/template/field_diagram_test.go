package template

import (
	"reflect"
	"testing"
)

func mappingTable() Field {
	return Field{Key: "mapping", Type: "table", Options: []any{
		map[string]any{"value": "bron_entiteit", "label": "Bron Entiteit", "type": "string"},
		map[string]any{"value": "bron_veld", "label": "Bron Veld", "type": "string"},
		map[string]any{"value": "doel_entiteit", "label": "Doel Entiteit", "type": "string"},
		map[string]any{"value": "doel_veld", "label": "Doel Veld", "type": "string"},
		map[string]any{"value": "tr_code", "label": "TR code", "type": "string"},
	}}
}

func diagramField(source string, cols map[string]string) Field {
	opts := []any{map[string]any{"value": DiagramSource, "label": source}}
	for _, role := range []string{DiagramFromEntity, DiagramFromAttr, DiagramToEntity, DiagramToAttr, DiagramLabel} {
		opts = append(opts, map[string]any{"value": role, "label": cols[role]})
	}
	return Field{Key: "lineage", Type: "diagram", Label: "Lineage", Options: opts}
}

var fullCols = map[string]string{
	DiagramFromEntity: "bron_entiteit", DiagramFromAttr: "bron_veld",
	DiagramToEntity: "doel_entiteit", DiagramToAttr: "doel_veld",
	DiagramLabel: "tr_code",
}

func TestDiagramField_IsVirtualAndKnown(t *testing.T) {
	if !IsKnownFieldType("diagram") || !IsVirtualFieldType("diagram") {
		t.Fatal("diagram must be a known virtual field type")
	}
}

func TestDiagramField_ValidBindingHasNoErrors(t *testing.T) {
	tpl := &Template{Fields: []Field{mappingTable(), diagramField("mapping", fullCols)}}
	for _, e := range Validate(tpl) {
		if len(e.Type) > 8 && e.Type[:8] == "diagram-" {
			t.Fatalf("unexpected %s: %s", e.Type, e.Message)
		}
	}
}

func TestDiagramField_AttributeAndLabelColumnsAreOptional(t *testing.T) {
	cols := map[string]string{DiagramFromEntity: "bron_entiteit", DiagramToEntity: "doel_entiteit"}
	tpl := &Template{Fields: []Field{mappingTable(), diagramField("mapping", cols)}}
	for _, e := range Validate(tpl) {
		if len(e.Type) > 8 && e.Type[:8] == "diagram-" {
			t.Fatalf("unexpected %s", e.Type)
		}
	}
}

func TestDiagramField_Errors(t *testing.T) {
	looped := mappingTable()
	cases := []struct {
		name   string
		fields []Field
		want   string
	}{
		{"missing source", []Field{mappingTable(), diagramField("", fullCols)}, "diagram-field-missing-source"},
		{"unknown source", []Field{mappingTable(), diagramField("ghost", fullCols)}, "diagram-field-unknown-source"},
		{"source not a table", []Field{{Key: "t", Type: "text"}, diagramField("t", fullCols)}, "diagram-field-source-not-table"},
		{"source inside a loop", []Field{
			{Key: "loop", Type: "loopstart"}, looped, {Key: "loop", Type: "loopstop"},
			diagramField("mapping", fullCols),
		}, "diagram-field-source-not-root"},
		{"missing from entity", []Field{mappingTable(), diagramField("mapping", map[string]string{DiagramToEntity: "doel_entiteit"})}, "diagram-field-missing-column"},
		{"missing to entity", []Field{mappingTable(), diagramField("mapping", map[string]string{DiagramFromEntity: "bron_entiteit"})}, "diagram-field-missing-column"},
		{"unknown column", []Field{mappingTable(), diagramField("mapping", map[string]string{
			DiagramFromEntity: "bron_entiteit", DiagramToEntity: "doel_entiteit", DiagramLabel: "nope",
		})}, "diagram-field-unknown-column"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if errs := Validate(&Template{Fields: c.fields}); !hasErr(errs, c.want) {
				t.Fatalf("want %s; got %+v", c.want, errs)
			}
		})
	}
}

func TestDiagramBindingOf_ResolvesColumnIndices(t *testing.T) {
	tpl := &Template{Fields: []Field{mappingTable(), diagramField("mapping", map[string]string{
		DiagramFromEntity: "bron_entiteit", DiagramToEntity: "doel_entiteit", DiagramToAttr: "doel_veld",
	})}}
	b, ok := DiagramBindingOf(tpl, "lineage")
	if !ok {
		t.Fatal("binding not resolved")
	}
	want := DiagramBinding{
		Projection: DiagramProjectionLineage,
		Source:     "mapping",
		Columns:    map[string]int{DiagramFromEntity: 0, DiagramFromAttr: -1, DiagramToEntity: 2, DiagramToAttr: 3, DiagramLabel: -1},
	}
	if !reflect.DeepEqual(b, want) {
		t.Fatalf("binding = %+v, want %+v", b, want)
	}
}

func TestDiagramBindingOf_UnknownOrMisboundFieldIsNotOK(t *testing.T) {
	tpl := &Template{Fields: []Field{mappingTable(), diagramField("ghost", fullCols), {Key: "x", Type: "text"}}}
	for _, key := range []string{"lineage", "x", "nope", ""} {
		if _, ok := DiagramBindingOf(tpl, key); ok {
			t.Fatalf("DiagramBindingOf(%q) ok, want false", key)
		}
	}
	if _, ok := DiagramBindingOf(nil, "lineage"); ok {
		t.Fatal("nil template must not resolve")
	}
}
