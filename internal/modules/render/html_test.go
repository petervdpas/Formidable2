package render

import (
	"strings"
	"testing"
)

func TestRenderHTML_Heading(t *testing.T) {
	got, err := RenderHTML("# Hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "<h1") || !strings.Contains(got, "Hello") {
		t.Errorf("got %q", got)
	}
}

func TestRenderHTML_StripsFrontmatter(t *testing.T) {
	src := "---\ntitle: x\n---\n# Body\n"
	got, _ := RenderHTML(src)
	if strings.Contains(got, "title:") {
		t.Errorf("frontmatter leaked into html: %q", got)
	}
	if !strings.Contains(got, "<h1") {
		t.Errorf("body missing: %q", got)
	}
}

func TestRenderHTML_FencedCode(t *testing.T) {
	src := "```go\nfunc main() {}\n```"
	got, _ := RenderHTML(src)
	if !strings.Contains(got, "<pre") {
		t.Errorf("missing <pre>: %q", got)
	}
	// chroma classes should appear
	if !strings.Contains(got, "class=") {
		t.Errorf("missing class attribute on highlighted code: %q", got)
	}
}

func TestRenderHTML_TagOutsideCode(t *testing.T) {
	src := "Look at #foo and #bar."
	got, _ := RenderHTML(src)
	if !strings.Contains(got, `inline-tag">#foo`) {
		t.Errorf("hashtag not decorated: %q", got)
	}
}

func TestRenderHTML_TagInsideCodeNotDecorated(t *testing.T) {
	src := "```\n#nope\n```"
	got, _ := RenderHTML(src)
	if strings.Contains(got, "inline-tag") {
		t.Errorf("hashtag inside code was decorated: %q", got)
	}
}

func TestRenderHTML_FileImage(t *testing.T) {
	src := "![logo](file:///abs/path/img.png)"
	got, _ := RenderHTML(src)
	if !strings.Contains(got, `src="file:///abs/path/img.png"`) {
		t.Errorf("file:// image not preserved: %q", got)
	}
}

func TestRenderHTML_TableExtension(t *testing.T) {
	src := "| a | b |\n| - | - |\n| 1 | 2 |\n"
	got, _ := RenderHTML(src)
	if !strings.Contains(got, "<table") {
		t.Errorf("GFM tables disabled? got %q", got)
	}
}

func TestRenderHTML_StrikethroughExtension(t *testing.T) {
	src := "~~bye~~"
	got, _ := RenderHTML(src)
	if !strings.Contains(got, "<del") && !strings.Contains(got, "<s>") {
		t.Errorf("GFM strikethrough disabled? got %q", got)
	}
}
