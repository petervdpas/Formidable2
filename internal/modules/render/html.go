package render

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

// goldmarkOnce keeps a single configured Markdown engine for the
// process — goldmark.Markdown is safe for concurrent use after Build.
var (
	goldmarkOnce sync.Once
	gm           goldmark.Markdown
)

func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Typographer,
			extension.Linkify,
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
				highlighting.WithFormatOptions(
					chromahtml.WithClasses(true),
					chromahtml.ClassPrefix("hljs-"),
				),
			),
		),
		goldmark.WithRendererOptions(
			goldhtml.WithUnsafe(), // allow inline <img> from convertFileImages
			goldhtml.WithHardWraps(),
		),
	)
}

func md() goldmark.Markdown {
	goldmarkOnce.Do(func() {
		gm = newMarkdown()
	})
	return gm
}

var (
	frontmatterStripRe = regexp.MustCompile(`(?s)\A---\n.+?\n---\n*`)
	imageFileRe        = regexp.MustCompile(`!\[([^\]]*)\]\((file://[^)]+)\)`)
	imageRelRe         = regexp.MustCompile(`!\[([^\]]*)\]\((images/[^)]+)\)`)
	tagRe              = regexp.MustCompile(`(^|\s)(#[\w.\-]+)`)
	preBlockRe         = regexp.MustCompile(`(?is)<pre[\s\S]*?</pre>|<code[\s\S]*?</code>`)
)

// stripFrontmatter — drops a leading YAML block before goldmark sees it.
func stripFrontmatter(s string) string {
	return frontmatterStripRe.ReplaceAllString(s, "")
}

// convertFileImages rewrites `![alt](file://…)` and `![alt](images/…)`
// to raw `<img>` so neither goldmark nor a future sanitizer rewrites
// the src. Keeps the same scheme/path the renderer received.
func convertFileImages(s string) string {
	s = imageFileRe.ReplaceAllString(s, `<img src="$2" alt="$1">`)
	s = imageRelRe.ReplaceAllString(s, `<img src="$2" alt="$1">`)
	return s
}

// decorateTagsOutsideCode wraps `#tag` in `<span class="inline-tag">…</span>`
// — but only outside `<pre>` and `<code>` blocks (so code samples that
// mention `#define` etc. aren't styled).
func decorateTagsOutsideCode(html string) string {
	parts := preBlockRe.Split(html, -1)
	matches := preBlockRe.FindAllString(html, -1)
	var b strings.Builder
	for i, seg := range parts {
		b.WriteString(tagRe.ReplaceAllString(seg, `$1<span class="inline-tag">$2</span>`))
		if i < len(matches) {
			b.WriteString(matches[i])
		}
	}
	return b.String()
}

// RenderHTML runs the markdown→HTML stage. Pipeline:
//  1. strip frontmatter
//  2. pre-rewrite `![alt](file://…)` / `images/…` to `<img>`
//  3. goldmark + chroma render
//  4. wrap stray `#tags` outside code blocks
func RenderHTML(markdown string) (string, error) {
	cleaned := stripFrontmatter(markdown)
	pre := convertFileImages(cleaned)

	var buf bytes.Buffer
	if err := md().Convert([]byte(pre), &buf); err != nil {
		return "", fmt.Errorf("render: html convert: %w", err)
	}
	return decorateTagsOutsideCode(buf.String()), nil
}
