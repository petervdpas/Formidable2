package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestRenderMarkdown_NoTemplate(t *testing.T) {
	tpl := &template.Template{}
	got, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "No template defined") {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_Simple(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `# {{title}}`,
		Fields: []template.Field{
			{Key: "title", Type: "text"},
		},
	}
	got, err := RenderMarkdown(map[string]any{"title": "Hello"}, tpl, &Options{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "# Hello" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_FieldHelper(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `Color: {{field "color"}}`,
		Fields: []template.Field{
			{Key: "color", Type: "dropdown", Options: []any{
				map[string]any{"value": "r", "label": "Red"},
			}},
		},
	}
	got, _ := RenderMarkdown(map[string]any{"color": "r"}, tpl, &Options{})
	if got != "Color: Red" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_Loop(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `{{#loop "items"}}- {{name}}{{/loop}}`,
		Fields: []template.Field{
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		},
	}
	got, _ := RenderMarkdown(map[string]any{
		"items": []any{
			map[string]any{"name": "a"},
			map[string]any{"name": "b"},
		},
	}, tpl, &Options{})
	if got != "- a\n- b" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_ParseError(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `{{#unclosed`,
	}
	_, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestRenderMarkdown_ImageStrategy(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `![logo]({{field "logo"}})`,
		Fields: []template.Field{
			{Key: "logo", Type: "image"},
		},
	}
	opts := &Options{
		ImageURL: func(name string) string { return "/storage/x/images/" + name },
	}
	got, _ := RenderMarkdown(map[string]any{"logo": "logo.png"}, tpl, opts)
	if got != "![logo](/storage/x/images/logo.png)" {
		t.Errorf("got %q", got)
	}
}
