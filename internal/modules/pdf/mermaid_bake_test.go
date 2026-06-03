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

func TestMermaidErrorSVG_IsValidSVGWithEscapedMessage(t *testing.T) {
	svg := mermaidErrorSVG("Parse error: got '<x>' & 'y'")
	if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("not a standalone svg: %.40q…", svg)
	}
	if !strings.Contains(svg, "Mermaid diagram error") {
		t.Fatalf("missing title: %q", svg)
	}
	if !strings.Contains(svg, "&lt;x&gt;") || !strings.Contains(svg, "&amp;") {
		t.Fatalf("message not XML-escaped: %q", svg)
	}
	if strings.Contains(svg, "<x>") {
		t.Fatalf("raw < leaked into svg: %q", svg)
	}
}

func TestBakeMermaidSVG_NoFencesReturnsUnchanged(t *testing.T) {
	// No fence -> early return before any browser/log use.
	var m Manager
	body := "# Title\n\nplain text, no diagrams"
	if got := m.bakeMermaidSVG(body, ""); got != body {
		t.Fatalf("body changed: %q", got)
	}
}
