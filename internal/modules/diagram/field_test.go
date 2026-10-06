package diagram

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func lineageTemplate() *template.Template {
	col := func(k string) map[string]any { return map[string]any{"value": k, "label": k, "type": "string"} }
	role := func(r, k string) map[string]any { return map[string]any{"value": r, "label": k} }
	return &template.Template{Fields: []template.Field{
		{Key: "mapping", Type: "table", Options: []any{col("be"), col("bv"), col("de"), col("dv"), col("tr")}},
		{Key: "lineage", Type: "diagram", Options: []any{
			role(template.DiagramSource, "mapping"),
			role(template.DiagramFromEntity, "be"), role(template.DiagramFromAttr, "bv"),
			role(template.DiagramToEntity, "de"), role(template.DiagramToAttr, "dv"),
			role(template.DiagramLabel, "tr"),
		}},
	}}
}

func TestRenderField_DrawsTheBoundTable(t *testing.T) {
	data := map[string]any{"mapping": []any{
		[]any{"Basisvariant", "Code", "Inschrijving", "Bk", "Q-01"},
	}}
	svg, ok := RenderField(lineageTemplate(), "lineage", data)
	if !ok || !strings.Contains(svg, ">Basisvariant<") || !strings.Contains(svg, ">Inschrijving<") {
		t.Fatalf("ok=%v svg=%.200s", ok, svg)
	}
}

func TestRenderField_NonStringCellsAreStringified(t *testing.T) {
	data := map[string]any{"mapping": []any{
		[]any{"A", float64(5578035), "T", true, nil},
	}}
	svg, _ := RenderField(lineageTemplate(), "lineage", data)
	if !strings.Contains(svg, ">5578035<") || !strings.Contains(svg, ">true<") {
		t.Fatalf("svg = %s", svg)
	}
}

func TestRenderField_EmptyOrMissingTableDrawsNothingButIsOK(t *testing.T) {
	for _, data := range []map[string]any{nil, {}, {"mapping": nil}, {"mapping": []any{}}, {"mapping": "garbage"}, {"mapping": []any{"not-a-row", 7}}} {
		svg, ok := RenderField(lineageTemplate(), "lineage", data)
		if !ok || svg != "" {
			t.Fatalf("data %v: ok=%v svg=%q, want ok and empty", data, ok, svg)
		}
	}
}

func TestRenderField_UnboundFieldIsNotOK(t *testing.T) {
	if _, ok := RenderField(lineageTemplate(), "mapping", nil); ok {
		t.Fatal("a table field is not a diagram")
	}
	if _, ok := RenderField(nil, "lineage", nil); ok {
		t.Fatal("nil template")
	}
}

func TestRenderField_AcceptsStringRows(t *testing.T) {
	data := map[string]any{"mapping": [][]string{{"A", "x", "T", "y", ""}}}
	if svg, _ := RenderField(lineageTemplate(), "lineage", data); !strings.Contains(svg, ">A<") {
		t.Fatalf("svg = %.200s", svg)
	}
}
