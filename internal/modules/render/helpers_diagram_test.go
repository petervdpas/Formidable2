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

// diagramOpts is a renderer for a saved record with a diagram-image writer that
// records what it was handed and answers ok.
type writtenImage struct{ name, svg string }

func diagramOpts(ok bool, written *[]writtenImage) *Options {
	return &Options{
		TemplateFilename: "m.yaml", Datafile: "r1.meta.json",
		DiagramImage: func(name, svg string) bool {
			*written = append(*written, writtenImage{name, svg})
			return ok
		},
	}
}

var oneMapping = map[string]any{"mapping": []any{[]any{"A", "x", "T", "y", "", ""}}}

func TestVirtualFieldHelper_DiagramWritesItsImageAndReferencesIt(t *testing.T) {
	var written []writtenImage
	tpl := diagramRenderTpl(`{{virtual-field "lineage" "image"}}`)
	tpl.Fields[1].Label = "Data [lineage]"
	got, err := RenderMarkdown(oneMapping, tpl, diagramOpts(true, &written))
	name := template.DiagramImageName("m.yaml", "r1.meta.json", "lineage")
	if err != nil || got != "![Data lineage](images/"+name+")" {
		t.Fatalf("got %q err %v", got, err)
	}
	if len(written) != 1 || written[0].name != name || !strings.HasPrefix(written[0].svg, "<svg") || !strings.Contains(written[0].svg, ">T<") {
		t.Fatalf("written = %+v, want the rendered SVG under %s", written, name)
	}
}

func TestVirtualFieldHelper_DiagramImageUsesTheImageResolver(t *testing.T) {
	var written []writtenImage
	opts := diagramOpts(true, &written)
	opts.ImageURL = func(n string) string { return "/img/" + n }
	got, _ := RenderMarkdown(oneMapping, diagramRenderTpl(`{{virtual-field "lineage" mode="image"}}`), opts)
	if want := "![lineage](/img/" + template.DiagramImageName("m.yaml", "r1.meta.json", "lineage") + ")"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestVirtualFieldHelper_DiagramFallsBackToInlineWhenTheImageCannotBeWritten(t *testing.T) {
	var written []writtenImage
	got, _ := RenderMarkdown(oneMapping, diagramRenderTpl(`{{virtual-field "lineage" "image"}}`), diagramOpts(false, &written))
	if !strings.Contains(got, "<svg ") || strings.Contains(got, "![") {
		t.Fatalf("a failed write must not leave a dangling reference: %.120q", got)
	}
}

func TestVirtualFieldHelper_ImageModeWithoutWriterOrRecordIsInline(t *testing.T) {
	for _, opts := range []*Options{
		{TemplateFilename: "m.yaml", Datafile: "r1.meta.json"},
		{TemplateFilename: "m.yaml", DiagramImage: func(string, string) bool { return true }},
	} {
		got, _ := RenderMarkdown(oneMapping, diagramRenderTpl(`{{virtual-field "lineage" "image"}}`), opts)
		if !strings.Contains(got, "<svg ") || strings.Contains(got, "![") {
			t.Fatalf("opts %+v: got %.120q", opts, got)
		}
	}
}

func TestVirtualFieldHelper_DiagramDefaultIsInlineAndWritesNothing(t *testing.T) {
	for _, md := range []string{`{{virtual-field "lineage"}}`, `{{virtual-field "lineage" "inline"}}`, `{{virtual-field "lineage" "bogus"}}`, `[{{virtual-field "lineage"}}]`} {
		var written []writtenImage
		data := oneMapping
		if strings.HasPrefix(md, "[") {
			data = map[string]any{}
		}
		got, _ := RenderMarkdown(data, diagramRenderTpl(md), diagramOpts(true, &written))
		if strings.Contains(got, "![") || len(written) != 0 {
			t.Fatalf("%s: got %.80q, written %d; only image mode touches files", md, got, len(written))
		}
		if data != nil && len(data) > 0 && !strings.Contains(got, "<svg ") {
			t.Fatalf("%s: no inline svg in %.80q", md, got)
		}
	}
}

func TestVirtualFieldHelper_EmptyDiagramEmitsNothingAndClearsItsFile(t *testing.T) {
	var written []writtenImage
	got, _ := RenderMarkdown(map[string]any{}, diagramRenderTpl(`[{{virtual-field "lineage" "image"}}]`), diagramOpts(true, &written))
	if got != "[]" {
		t.Fatalf("got %q", got)
	}
	if len(written) != 1 || written[0].svg != "" {
		t.Fatalf("written = %+v, want one empty write so a stale file is removed", written)
	}
}

func TestVirtualFieldHelper_ImageModeMarkdownNeverCarriesAVersion(t *testing.T) {
	var written []writtenImage
	opts := diagramOpts(true, &written)
	opts.ImageURL = func(n string) string { return "/api/images/m/" + n }
	got, _ := RenderMarkdown(oneMapping, diagramRenderTpl(`{{virtual-field "lineage" "image"}}`), opts)
	name := template.DiagramImageName("m.yaml", "r1.meta.json", "lineage")
	if got != "![lineage](/api/images/m/"+name+")" {
		t.Fatalf("markdown is the portable output and must hold the plain file reference: %q", got)
	}
	if v := opts.diagramImages["/api/images/m/"+name]; len(v) != 8 {
		t.Fatalf("the render must record the stored image's content version, got %q", v)
	}
}
