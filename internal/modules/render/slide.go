package render

import (
	"fmt"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// renderSlide turns a slide document into positioned HTML. Each block is
// rendered by the same emitter its kind uses everywhere else (DRY): markdown
// through goldmark, mermaid to a fenced diagram, image/table/list to their
// markup. Each block's HTML is wrapped in an absolutely-positioned box on the
// canvas (size from the field's deck-wide options); z-order is the block's
// order in the document. Empty doc -> "".
func renderSlide(v any, f *template.Field, opts *Options) string {
	doc, err := template.ParseSlideDoc(v)
	if err != nil || len(doc.Blocks) == 0 {
		return ""
	}
	w, h := template.SlideCanvasWidth, template.SlideCanvasHeight
	if f != nil {
		w, h = template.SlideCanvasSize(*f)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb,
		`<div class="slide-canvas" style="position:relative;width:%dpx;height:%dpx">`,
		w, h)
	for _, b := range doc.Blocks {
		inner, _ := RenderHTML(emitSlideBlock(b.Kind, b.Content, b.Lang, opts))
		cls := "slide-block"
		if b.Fragment != "" {
			cls += " fragment " + b.Fragment
		}
		fmt.Fprintf(&sb,
			`<div class="%s" style="position:absolute;left:%dpx;top:%dpx;width:%dpx;height:%dpx;%s">%s</div>`,
			cls, b.X, b.Y, b.W, b.H, b.InlineStyle(), inner)
	}
	sb.WriteString("</div>")
	return sb.String()
}

// RenderSlideBlockHTML renders one block's content to HTML using the same
// per-kind emitter the full slide uses, so the canvas editor previews exactly
// what the deck will render. templateName scopes image URLs.
func (m *Manager) RenderSlideBlockHTML(templateName, kind string, content any) string {
	opts := m.optionsFor(templateName, "")
	html, _ := RenderHTML(emitSlideBlock(kind, content, "", opts))
	return html
}

// emitSlideBlock renders one reveal block's content to markdown/HTML, dispatching
// on the reveal element kind. text renders as markdown; code/math/quote map to
// their markdown/reveal forms; video/embed emit reveal media elements; image/
// table/list/mermaid reuse the shared emitters. lang is the code language.
func emitSlideBlock(kind string, content any, lang string, opts *Options) string {
	switch kind {
	case "mermaid":
		return emitMermaid(content)
	case "image":
		if url := emitImage(content, opts); url != "" {
			return "![](" + url + ")"
		}
		return ""
	case "table":
		return slideTableMarkdown(content)
	case "list":
		return emitList(content)
	case "quote":
		return blockquoteMarkdown(stringify(content))
	case "code":
		return "```" + strings.TrimSpace(lang) + "\n" + stringify(content) + "\n```"
	case "math":
		src := strings.TrimSpace(stringify(content))
		if src == "" {
			return ""
		}
		return `\[` + src + `\]` // reveal's math plugin renders this
	case "video":
		url := strings.TrimSpace(stringify(content))
		if url == "" {
			return ""
		}
		return `<video controls data-autoplay src="` + url + `"></video>`
	case "embed":
		url := strings.TrimSpace(stringify(content))
		if url == "" {
			return ""
		}
		return `<iframe data-src="` + url + `" allowfullscreen></iframe>`
	default: // text (markdown) and anything else
		return stringify(content)
	}
}

// blockquoteMarkdown prefixes every line with "> " so goldmark builds a real
// blockquote (reveal styles it as a quote).
func blockquoteMarkdown(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = "> " + ln
	}
	return strings.Join(lines, "\n")
}

// slideTableMarkdown renders a table block's 2D array as a GFM table, treating
// the first row as the header and inserting the delimiter row goldmark needs to
// recognise it as a table (emitTable omits that, so its output renders as a
// paragraph). Empty or non-array content yields nothing.
func slideTableMarkdown(v any) string {
	rows, ok := v.([]any)
	if !ok || len(rows) == 0 {
		return ""
	}
	line := func(r any) string {
		cells, _ := r.([]any)
		strs := make([]string, len(cells))
		for i, c := range cells {
			strs[i] = stringify(c)
		}
		return "| " + strings.Join(strs, " | ") + " |"
	}
	header, _ := rows[0].([]any)
	n := len(header)
	if n == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(line(rows[0]))
	sb.WriteString("\n|")
	for range n {
		sb.WriteString(" --- |")
	}
	for _, r := range rows[1:] {
		sb.WriteString("\n")
		sb.WriteString(line(r))
	}
	return sb.String()
}
