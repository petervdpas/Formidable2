package diagram

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func lineageTemplate() *template.Template {
	return &template.Template{Fields: []template.Field{
		{Key: "mapping", Type: "table", Format: template.TablePatternDataLineage, Options: template.ApplyTablePattern(nil, template.TablePatternDataLineage)},
		{Key: "lineage", Type: "diagram", Options: []any{
			map[string]any{"value": template.DiagramSource, "label": "mapping"},
		}},
	}}
}

func TestRenderField_DrawsTheBoundTable(t *testing.T) {
	data := map[string]any{"mapping": []any{
		[]any{"Basisvariant", "Code", "Inschrijving", "Bk", "", "Q-01"},
	}}
	svg, ok := RenderField(lineageTemplate(), "lineage", data)
	if !ok || !strings.Contains(svg, ">Basisvariant<") || !strings.Contains(svg, ">Inschrijving<") {
		t.Fatalf("ok=%v svg=%.200s", ok, svg)
	}
}

func TestRenderField_NonStringCellsAreStringified(t *testing.T) {
	data := map[string]any{"mapping": []any{
		[]any{"A", float64(5578035), "T", true, nil, nil},
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
	data := map[string]any{"mapping": [][]string{{"A", "x", "T", "y", "", ""}}}
	if svg, _ := RenderField(lineageTemplate(), "lineage", data); !strings.Contains(svg, ">A<") {
		t.Fatalf("svg = %.200s", svg)
	}
}

func referenceLineageTemplate(target string) *template.Template {
	t := lineageTemplate()
	opts := t.Fields[0].Options
	opts[5].(map[string]any)[template.ReferenceTargetKey] = target
	t.Fields = append(t.Fields,
		template.Field{Key: "rules", Type: "loopstart"},
		template.Field{Key: "tr_code", Type: "text"},
		template.Field{Key: "rules", Type: "loopstop"},
	)
	return t
}

func TestRenderField_RulePillsLinkToDefinedRules(t *testing.T) {
	data := map[string]any{
		"mapping": []any{[]any{"", "", "T", "a", "", "Q-01, X-99"}},
		"rules":   []any{map[string]any{"tr_code": "q-01"}},
	}
	svg, ok := RenderField(referenceLineageTemplate("tr_code"), "lineage", data)
	if !ok {
		t.Fatal("not ok")
	}
	if !strings.Contains(svg, `<a href="#ref-tr_code-q-01">`) {
		t.Fatalf("defined rule not linked: %s", svg)
	}
	if strings.Contains(svg, `href="#ref-tr_code-x-99"`) || !strings.Contains(svg, `data-class="undefined"`) {
		t.Fatalf("undefined rule must be flagged, not linked: %s", svg)
	}
}

func TestRenderField_PlainRuleColumnHasNoLinksOrFlags(t *testing.T) {
	data := map[string]any{"mapping": []any{[]any{"", "", "T", "a", "", "Q-01"}}}
	svg, _ := RenderField(lineageTemplate(), "lineage", data)
	if strings.Contains(svg, "href=") || strings.Contains(svg, `data-class="undefined"`) {
		t.Fatalf("no reference target, no links: %s", svg)
	}
}

func TestRenderField_RuleSetPartsLinkToDefinedRules(t *testing.T) {
	data := map[string]any{
		"mapping": []any{[]any{"A", "x", "T", "y", "", "Q-01, X-99"}},
		"rules":   []any{map[string]any{"tr_code": "Q-01"}},
	}
	svg, _ := RenderField(referenceLineageTemplate("tr_code"), "lineage", data)
	if !strings.Contains(svg, `<a href="#ref-tr_code-q-01"><g data-part="Q-01"`) || !strings.Contains(svg, `data-part="X-99" data-class="undefined"`) {
		t.Fatalf("got %s", svg)
	}
}

func twoTableTemplate(withRelations bool) *template.Template {
	t := lineageTemplate()
	t.Fields = append(t.Fields, template.Field{
		Key: "rels", Type: "table", Format: template.TablePatternEntityRelations,
		Options: template.ApplyTablePattern(nil, template.TablePatternEntityRelations),
	})
	if withRelations {
		t.Fields[1].Options = append(t.Fields[1].Options, map[string]any{"value": template.DiagramRelations, "label": "rels"})
	}
	return t
}

func twoTableData() map[string]any {
	return map[string]any{
		"mapping": []any{[]any{"Account", "Pcn", "Medewerker", "Personeelsnummer", "", ""}},
		"rels": []any{
			[]any{"target", "RequestContext", "", "Medewerker", "", "bevat"},
			[]any{"source", "Account", "AfdelingId", "Afdeling", "Id", ""},
		},
	}
}

func TestRenderField_SecondTableFeedsRelationsIntoTheLineageDiagram(t *testing.T) {
	svg, ok := RenderField(twoTableTemplate(true), "lineage", twoTableData())
	if !ok {
		t.Fatal("not ok")
	}
	for _, want := range []string{">Account<", ">Medewerker<", ">RequestContext<", ">Afdeling<", ">bevat<", `data-relation="1"`, `data-edge="0"`} {
		if !strings.Contains(svg, want) {
			t.Fatalf("missing %q in %s", want, svg)
		}
	}
}

func TestRenderField_RelationsTableIsOptional(t *testing.T) {
	svg, ok := RenderField(twoTableTemplate(false), "lineage", twoTableData())
	if !ok || strings.Contains(svg, "data-relation=") || strings.Contains(svg, ">RequestContext<") {
		t.Fatalf("an unbound relations table must not be drawn: ok=%v %s", ok, svg)
	}
	plain, _ := RenderField(lineageTemplate(), "lineage", twoTableData())
	if svg != plain {
		t.Fatal("a diagram without a relations table must render exactly as before")
	}
}

func TestRenderField_EmptyRelationsTableChangesNothing(t *testing.T) {
	data := twoTableData()
	data["rels"] = []any{}
	with, _ := RenderField(twoTableTemplate(true), "lineage", data)
	plain, _ := RenderField(lineageTemplate(), "lineage", data)
	if with != plain {
		t.Fatal("an empty relations table must leave the lineage diagram unchanged")
	}
}

func TestRenderField_RelationsTableAsSourceDrawsOnlyRelations(t *testing.T) {
	tpl := twoTableTemplate(false)
	tpl.Fields[1].Options = []any{map[string]any{"value": template.DiagramSource, "label": "rels"}}
	svg, ok := RenderField(tpl, "lineage", twoTableData())
	if !ok || !strings.Contains(svg, ">RequestContext<") || !strings.Contains(svg, "data-relation=") {
		t.Fatalf("ok=%v %s", ok, svg)
	}
	if strings.Contains(svg, "data-edge=") || strings.Contains(svg, ">Pcn<") {
		t.Fatalf("the mapping table is not this diagram's source: %s", svg)
	}
	if empty, ok := RenderField(tpl, "lineage", map[string]any{}); !ok || empty != "" {
		t.Fatalf("empty relations: ok=%v %q", ok, empty)
	}
}

func TestRenderField_RelationsOnlyEntitiesAppearEvenWithoutMappings(t *testing.T) {
	data := map[string]any{"rels": []any{[]any{"source", "A", "", "B", "", ""}}}
	svg, ok := RenderField(twoTableTemplate(true), "lineage", data)
	if !ok || !strings.Contains(svg, ">A<") || !strings.Contains(svg, ">B<") {
		t.Fatalf("ok=%v %q", ok, svg)
	}
}
