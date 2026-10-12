package template

import (
	"reflect"
	"testing"
)

var relationKeys = []string{"side", "from_entity", "from_attribute", "to_entity", "to_attribute", "label"}

func relationsTable(key string) Field {
	return Field{Key: key, Type: "table", Format: TablePatternEntityRelations, Options: ApplyTablePattern(nil, TablePatternEntityRelations)}
}

func diagramWith(source, relations string) Field {
	opts := []any{map[string]any{"value": DiagramSource, "label": source}}
	if relations != "" {
		opts = append(opts, map[string]any{"value": DiagramRelations, "label": relations})
	}
	return Field{Key: "lineage", Type: "diagram", Options: opts}
}

func TestEntityRelationsPattern_ColumnsAndDiagramCounterpart(t *testing.T) {
	var p *TablePatternDescriptor
	for _, d := range TablePatterns() {
		if d.ID == TablePatternEntityRelations {
			p = &d
		}
	}
	if p == nil || p.Diagram != DiagramProjectionRelations || p.Shape == nil {
		t.Fatalf("pattern = %+v", p)
	}
	var keys []string
	for _, r := range p.Shape.Rows {
		keys = append(keys, r.Defaults["value"].(string))
	}
	if !reflect.DeepEqual(keys, relationKeys) {
		t.Fatalf("keys = %v", keys)
	}
	if p.Shape.Rows[0].Defaults["type"] != "dropdown" || p.Shape.Rows[1].Defaults["type"] != "string" {
		t.Fatalf("side must be a dropdown, the rest strings: %+v", p.Shape.Rows[:2])
	}
}

func TestEntityRelationsPattern_SideChoicesKeepValuesAndUserLabels(t *testing.T) {
	side := func(opts []any) string { return opts[0].(map[string]any)["choices"].(string) }
	if got := side(ApplyTablePattern(nil, TablePatternEntityRelations)); got != "source:Source|target:Target" {
		t.Fatalf("default choices = %q", got)
	}
	in := []any{map[string]any{"value": "side", "label": "Kant", "choices": "target:Doel|bogus:X|source:Bron"}}
	if got := side(ApplyTablePattern(in, TablePatternEntityRelations)); got != "source:Bron|target:Doel" {
		t.Fatalf("choices = %q, want fixed values with the author's labels", got)
	}
	labels := map[string]string{"side": "Kant", "side.source": "Bron", "side.target": "Doel"}
	if got := side(ApplyTablePatternLabels(nil, TablePatternEntityRelations, labels)); got != "source:Bron|target:Doel" {
		t.Fatalf("seeded choices = %q, want translated labels", got)
	}
}

func TestDiagramField_OptionRowsAreSourceAndRelations(t *testing.T) {
	rows := fieldDescriptors["diagram"].OptionsShape.Rows
	if len(rows) != 2 || rows[0].Defaults["value"] != DiagramSource || rows[1].Defaults["value"] != DiagramRelations ||
		rows[0].Input != "table-field" || rows[1].Input != "relations-table" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestDiagramBinding_TwoTables(t *testing.T) {
	tpl := &Template{Fields: []Field{lineageTable("mapping"), relationsTable("rels"), diagramWith("mapping", "rels")}}
	if got := diagramErrs(Validate(tpl)); len(got) != 0 {
		t.Fatalf("unexpected %v", got)
	}
	b, ok := DiagramBindingOf(tpl, "lineage")
	if !ok || b.Projection != DiagramProjectionLineage || b.Source != "mapping" || b.Relations != "rels" {
		t.Fatalf("binding = %+v ok=%v", b, ok)
	}
	want := map[string]int{DiagramSide: 0, DiagramFromEntity: 1, DiagramFromAttr: 2, DiagramToEntity: 3, DiagramToAttr: 4, DiagramLabel: 5, DiagramExample: -1}
	if !reflect.DeepEqual(b.RelationColumns, want) {
		t.Fatalf("relation columns = %v, want %v", b.RelationColumns, want)
	}
}

func TestDiagramBinding_RelationsTableAloneIsItsOwnDiagram(t *testing.T) {
	tpl := &Template{Fields: []Field{relationsTable("rels"), diagramWith("rels", "")}}
	if got := diagramErrs(Validate(tpl)); len(got) != 0 {
		t.Fatalf("unexpected %v", got)
	}
	b, ok := DiagramBindingOf(tpl, "lineage")
	if !ok || b.Projection != DiagramProjectionRelations || b.Source != "rels" || b.Relations != "" {
		t.Fatalf("binding = %+v ok=%v", b, ok)
	}
	if b.Columns[DiagramSide] != 0 || b.Columns[DiagramFromEntity] != 1 {
		t.Fatalf("columns = %v", b.Columns)
	}
}

func TestDiagramField_RelationsErrors(t *testing.T) {
	cases := []struct {
		name   string
		fields []Field
		want   string
	}{
		{"unknown relations table", []Field{lineageTable("mapping"), diagramWith("mapping", "ghost")}, "diagram-field-unknown-relations"},
		{"relations is not a table", []Field{lineageTable("mapping"), {Key: "t", Type: "text"}, diagramWith("mapping", "t")}, "diagram-field-relations-not-pattern"},
		{"relations table has another pattern", []Field{lineageTable("mapping"), lineageTable("other"), diagramWith("mapping", "other")}, "diagram-field-relations-not-pattern"},
		{"relations same as source", []Field{relationsTable("rels"), diagramWith("rels", "rels")}, "diagram-field-relations-same-as-source"},
		{"relations inside a loop", []Field{
			lineageTable("mapping"),
			{Key: "L", Type: "loopstart"}, relationsTable("rels"), {Key: "L", Type: "loopstop"},
			diagramWith("mapping", "rels"),
		}, "diagram-field-relations-not-root"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := diagramErrs(Validate(&Template{Fields: c.fields})); !reflect.DeepEqual(got, []string{c.want}) {
				t.Fatalf("got %v, want [%s]", got, c.want)
			}
		})
	}
}

func TestDiagramTableKeys_IncludesRelationsTables(t *testing.T) {
	fields := []Field{lineageTable("a"), relationsTable("r"), {Key: "plain", Type: "table"}}
	if got := DiagramTableKeys(fields); !reflect.DeepEqual(got, []string{"a", "r"}) {
		t.Fatalf("got %v", got)
	}
}
