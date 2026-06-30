package render

import (
	"strings"
	"testing"
)

func TestRenderSlide_PositionsAndBlockKinds(t *testing.T) {
	doc := map[string]any{"blocks": []any{
		map[string]any{"id": "b1", "kind": "textarea", "content": "## Hello",
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
	html := renderSlide(doc, opts)

	for _, want := range []string{
		`class="slide-canvas"`,
		`width:1280px;height:720px`,
		`left:40px;top:60px;width:600px;height:200px`, // markdown block geometry
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
	if got := renderSlide(map[string]any{"blocks": []any{}}, &Options{}); got != "" {
		t.Errorf("empty slide should render nothing, got %q", got)
	}
	if got := renderSlide(nil, &Options{}); got != "" {
		t.Errorf("nil slide should render nothing, got %q", got)
	}
}
