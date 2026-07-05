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

func TestRenderSlide_ShapeBlockEmitsSVG(t *testing.T) {
	doc := map[string]any{"blocks": []any{
		map[string]any{"id": "s1", "kind": "shape",
			"content": map[string]any{"shape": "ellipse", "fill": "#ff0000", "stroke": "#00ff00", "strokeWidth": float64(4)},
			"x":       float64(10), "y": float64(10), "w": float64(200), "h": float64(120)},
	}}
	html := renderSlide(doc, nil, &Options{})
	for _, want := range []string{
		`class="slide-block slide-block-shape"`, // per-kind CSS hook
		`<svg class="slide-shape"`,              // raw SVG survives goldmark
		`<ellipse`,                              // chosen variant
		`fill="#ff0000"`, `stroke="#00ff00"`, `stroke-width="4"`,
		`vector-effect="non-scaling-stroke"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("shape render missing %q\n---\n%s", want, html)
		}
	}
}

func TestEmitShape_SanitizesAndDefaults(t *testing.T) {
	// Unknown variant + hostile color + out-of-range stroke fall back to defaults;
	// a rectangle is the default variant.
	got := emitShape(map[string]any{
		"shape": "banana", "fill": `"><script>`, "stroke": "red", "strokeWidth": float64(999),
	})
	if !strings.Contains(got, "<rect") {
		t.Errorf("unknown variant should fall back to rectangle\n%s", got)
	}
	if strings.Contains(got, "<script") || strings.Contains(got, "script>") {
		t.Errorf("hostile color must not survive into the SVG\n%s", got)
	}
	if !strings.Contains(got, `fill="#3b82f6"`) {
		t.Errorf("rejected fill should fall back to default\n%s", got)
	}
	if !strings.Contains(got, `stroke="red"`) {
		t.Errorf("a bare CSS color name should be accepted\n%s", got)
	}
	if !strings.Contains(got, `stroke-width="2"`) {
		t.Errorf("out-of-range stroke width should fall back to default\n%s", got)
	}
}

func TestEmitShape_LineDirection(t *testing.T) {
	// A line with no direction defaults to horizontal (across the box centre),
	// not the box diagonal.
	if got := emitShape(map[string]any{"shape": "line"}); !strings.Contains(got, `x1="0" y1="50" x2="100" y2="50"`) {
		t.Errorf("default line should be horizontal\n%s", got)
	}
	if got := emitShape(map[string]any{"shape": "line", "direction": "vertical"}); !strings.Contains(got, `x1="50" y1="0" x2="50" y2="100"`) {
		t.Errorf("vertical line endpoints wrong\n%s", got)
	}
	// An arrow honors direction and keeps its marker.
	got := emitShape(map[string]any{"shape": "arrow", "direction": "diagonal-up"})
	if !strings.Contains(got, `x1="0" y1="100" x2="100" y2="0"`) || !strings.Contains(got, "marker-end") {
		t.Errorf("diagonal-up arrow wrong\n%s", got)
	}
}

func TestRenderSlide_ImportedSVGRendersAsImage(t *testing.T) {
	// An imported SVG is stored as a file and rendered as an <img> through the
	// image URL resolver, not as inline markup.
	doc := map[string]any{"blocks": []any{
		map[string]any{"id": "s1", "kind": "shape",
			"content": map[string]any{"svgFile": "shape-1.svg"},
			"x":       float64(0), "y": float64(0), "w": float64(200), "h": float64(120)},
	}}
	opts := &Options{ImageURL: func(name string) string { return "/api/images/tpl/" + name }}
	html := renderSlide(doc, nil, opts)
	if !strings.Contains(html, `<img`) || !strings.Contains(html, "/api/images/tpl/shape-1.svg") {
		t.Errorf("imported SVG should render as an <img> to the images route\n%s", html)
	}
}

func TestRenderSlide_TintedSVGUsesMask(t *testing.T) {
	// A tinted (single-colour) SVG recolours via a CSS mask over a solid fill,
	// not an <img>, so the whole shape takes the chosen colour.
	doc := map[string]any{"blocks": []any{
		map[string]any{"id": "s1", "kind": "shape",
			"content": map[string]any{"svgFile": "icon.svg", "tint": "#ff0000"},
			"x":       float64(0), "y": float64(0), "w": float64(80), "h": float64(80)},
	}}
	opts := &Options{ImageURL: func(name string) string { return "/api/images/tpl/" + name }}
	html := renderSlide(doc, nil, opts)
	for _, want := range []string{`class="slide-shape-tint"`, "background-color:#ff0000", "mask-image:url(/api/images/tpl/icon.svg)"} {
		if !strings.Contains(html, want) {
			t.Errorf("tinted SVG render missing %q\n%s", want, html)
		}
	}
	// A rejected tint colour falls back to the plain <img>.
	doc2 := map[string]any{"blocks": []any{
		map[string]any{"id": "s2", "kind": "shape",
			"content": map[string]any{"svgFile": "icon.svg", "tint": `"><script>`},
			"x":       float64(0), "y": float64(0), "w": float64(80), "h": float64(80)},
	}}
	if h := renderSlide(doc2, nil, opts); strings.Contains(h, "slide-shape-tint") || strings.Contains(h, "script") {
		t.Errorf("hostile tint must be rejected, falling back to <img>\n%s", h)
	}
}

func TestEmitShape_TriangleAndNoFill(t *testing.T) {
	// Triangle is a filled polygon; fill="none" gives an outline-only shape.
	got := emitShape(map[string]any{"shape": "triangle", "fill": "none", "stroke": "#000000"})
	if !strings.Contains(got, "<polygon") {
		t.Errorf("triangle should be a polygon\n%s", got)
	}
	if !strings.Contains(got, `fill="none"`) {
		t.Errorf("no-fill triangle should keep fill=none\n%s", got)
	}
	// A rectangle can also be outline-only.
	if r := emitShape(map[string]any{"shape": "rectangle", "fill": "none"}); !strings.Contains(r, `fill="none"`) {
		t.Errorf("no-fill rectangle should keep fill=none\n%s", r)
	}
}
