package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestRenderSlide_PositionsAndBlockKinds(t *testing.T) {
	doc := map[string]any{"blocks": []any{
		map[string]any{"id": "b1", "kind": "text", "content": "## Hello",
			"x": float64(40), "y": float64(60), "w": float64(600), "h": float64(200)},
		map[string]any{"id": "b2", "kind": "mermaid", "content": "graph TD; A-->B",
			"x": float64(700), "y": float64(60), "w": float64(500), "h": float64(300)},
		map[string]any{"id": "b3", "kind": "image", "content": "pic.png",
			"x": float64(40), "y": float64(300), "w": float64(300), "h": float64(200)},
		map[string]any{"id": "b4", "kind": "table",
			"content": []any{[]any{"A", "B"}, []any{"1", "2"}},
			"x":       float64(700), "y": float64(400), "w": float64(500), "h": float64(200)},
	}}
	opts := &Options{ImageURL: func(name string) string { return "/img/" + name }}
	html := renderSlide(doc, nil, opts)

	for _, want := range []string{
		`class="slide-canvas"`,
		`width:1280px;height:720px`,
		`class="slide-block slide-block-image"`, // per-kind class drives media sizing
		`left:40px;top:60px;width:600px;height:200px`, // block geometry from stored values
		`<h2`,                  // markdown rendered through goldmark
		`<pre class="mermaid"`, // mermaid block
		`/img/pic.png`,         // image resolved via ImageURL
		`<table`,               // table matrix rendered (not option-label)
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered slide missing %q\n---\n%s", want, html)
		}
	}
}

func TestRenderSlide_EmptyIsBlank(t *testing.T) {
	if got := renderSlide(map[string]any{"blocks": []any{}}, nil, &Options{}); got != "" {
		t.Errorf("empty slide should render nothing, got %q", got)
	}
	if got := renderSlide(nil, nil, &Options{}); got != "" {
		t.Errorf("nil slide should render nothing, got %q", got)
	}
}

func TestRenderSlide_HonorsCanvasSizeOption(t *testing.T) {
	doc := map[string]any{"blocks": []any{
		map[string]any{"id": "b", "kind": "text", "content": "hi",
			"x": float64(0), "y": float64(0), "w": float64(100), "h": float64(80)},
	}}
	f := &template.Field{Type: "slide", Options: []any{
		map[string]any{"value": "canvas_width", "label": "1920"},
		map[string]any{"value": "canvas_height", "label": "1080"},
	}}
	html := renderSlide(doc, f, &Options{})
	if !strings.Contains(html, "width:1920px;height:1080px") {
		t.Errorf("expected authored canvas size 1920x1080\n%s", html)
	}
}
