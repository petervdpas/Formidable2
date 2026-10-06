package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func diagramRenderTpl(md string) *template.Template {
	return &template.Template{
		Name: "m", Filename: "m.yaml", MarkdownTemplate: md,
		Fields: []template.Field{
			{Key: "mapping", Type: "table", Format: template.TablePatternDataLineage, Options: template.ApplyTablePattern(nil, template.TablePatternDataLineage)},
			{Key: "lineage", Type: "diagram", Options: []any{
				map[string]any{"value": template.DiagramSource, "label": "mapping"},
			}},
		},
	}
}

func TestVirtualFieldHelper_DiagramEmitsSVGBlock(t *testing.T) {
	data := map[string]any{"mapping": []any{[]any{"Basisvariant", "Code", "Inschrijving", "Bk"}}}
	got, err := RenderMarkdown(data, diagramRenderTpl("before\n\n{{virtual-field \"lineage\"}}\n\nafter"), &Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `<div class="formidable-diagram">`+"\n<svg ") || !strings.Contains(got, ">Basisvariant<") {
		t.Fatalf("no diagram block in %q", got)
	}
	if strings.Contains(got, "&lt;svg") {
		t.Fatal("svg was HTML-escaped")
	}
}

func TestVirtualFieldHelper_DiagramSurvivesHTMLPipeline(t *testing.T) {
	data := map[string]any{"mapping": []any{[]any{"A", "x", "T", "y"}}}
	md, err := RenderMarkdown(data, diagramRenderTpl("{{virtual-field \"lineage\"}}"), &Options{})
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTML(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<svg ") || strings.Contains(html, "<p><svg") {
		t.Fatalf("svg not passed through as a block: %.300s", html)
	}
}

func TestVirtualFieldHelper_DiagramEmptyTableEmitsNothing(t *testing.T) {
	got, err := RenderMarkdown(map[string]any{}, diagramRenderTpl("[{{virtual-field \"lineage\"}}]"), &Options{})
	if err != nil || got != "[]" {
		t.Fatalf("got %q err %v, want []", got, err)
	}
}
