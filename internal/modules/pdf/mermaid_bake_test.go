package pdf

import (
	"strings"
	"testing"
)

func TestMermaidFenceRe_CapturesSource(t *testing.T) {
	body := "intro\n\n```mermaid\nflowchart TD\n  A-->B\n```\n\noutro"
	locs := mermaidFenceRe.FindAllStringSubmatchIndex(body, -1)
	if len(locs) != 1 {
		t.Fatalf("matches = %d, want 1", len(locs))
	}
	src := body[locs[0][2]:locs[0][3]]
	if src != "flowchart TD\n  A-->B" {
		t.Fatalf("source = %q, want the diagram body", src)
	}
}

func TestMermaidFenceRe_MatchesMultiple(t *testing.T) {
	body := "```mermaid\ngantt\n```\ntext\n```mermaid\npie\n```"
	if got := len(mermaidFenceRe.FindAllStringSubmatchIndex(body, -1)); got != 2 {
		t.Fatalf("matches = %d, want 2", got)
	}
}

func TestMermaidFenceRe_IgnoresOtherFences(t *testing.T) {
	body := "```go\nfmt.Println()\n```"
	if got := len(mermaidFenceRe.FindAllStringSubmatchIndex(body, -1)); got != 0 {
		t.Fatalf("matches = %d, want 0 (non-mermaid fence)", got)
	}
}

func TestMermaidErrorSVG_IsBombWithTitle(t *testing.T) {
	svg := mermaidErrorSVG()
	if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("not a standalone svg: %.40q…", svg)
	}
	if !strings.Contains(svg, "Syntax error in mermaid") {
		t.Fatalf("missing title: %q", svg)
	}
	if !strings.Contains(svg, "<path d=") {
		t.Fatalf("missing bomb glyph: %q", svg)
	}
}

func TestBakeMermaidSVG_NoFencesReturnsUnchanged(t *testing.T) {
	// No fence -> early return before any browser/log use.
	var m Manager
	body := "# Title\n\nplain text, no diagrams"
	if got := m.bakeMermaidSVG(body, "", 0); got != body {
		t.Fatalf("body changed: %q", got)
	}
}

func TestScaleSVGWidth_PinsWidthAndAspectHeight(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="800" height="400" viewBox="0 0 800 400" style="max-width: 800px;"><g/></svg>`
	got := scaleSVGWidth(svg, 400)
	if !strings.Contains(got, `width="400"`) {
		t.Fatalf("width not pinned: %q", got)
	}
	if !strings.Contains(got, `height="200"`) {
		t.Fatalf("height not aspect-scaled to 200: %q", got)
	}
	if !strings.Contains(got, "max-width:400px") {
		t.Fatalf("inline max-width not rewritten: %q", got)
	}
	if !strings.HasSuffix(got, "<g/></svg>") {
		t.Fatalf("body mangled: %q", got)
	}
}

func TestScaleSVGWidth_ZeroIsNoOp(t *testing.T) {
	svg := `<svg width="800" height="400" viewBox="0 0 800 400"></svg>`
	if got := scaleSVGWidth(svg, 0); got != svg {
		t.Fatalf("zero width changed svg: %q", got)
	}
}
