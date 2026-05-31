package render

import (
	_ "embed"
	"fmt"
	"html"
	"strings"
)

// proseStylesheet is the wiki-prose CSS, embedded at build time so
// RenderFullHTML is self-contained. The same file is imported by Vite
// into the SPA bundle (frontend/src/styles/index.css), keeping preview
// and exported HTML identical.
//
// Syntax-highlight token colors live INSIDE this CSS ("Chroma github
// style" section). If html.go switches off WithStyle("github"),
// regenerate that block; there is no runtime CSS generation.
//
//go:embed assets/formidable-prose.css
var proseStylesheet string

// ProseCSS returns the embedded `formidable-prose` stylesheet, for
// consumers that produce their own HTML envelopes (wiki HTTP server,
// export tools) and want the same CSS without re-embedding it.
func ProseCSS() string { return proseStylesheet }

// RenderFullHTML returns a self-contained HTML document: full scaffolding,
// a <title> from frontmatter (falling back to the datafile stem, then
// "Untitled"), the inlined prose stylesheet, and the rendered body.
func (m *Manager) RenderFullHTML(templateName, datafile string) (string, error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return "", fmt.Errorf("render: load template %q: %w", templateName, err)
	}
	values := map[string]any{}
	if datafile != "" {
		if loaded := m.storage.LoadForm(templateName, datafile); loaded != nil {
			values = loaded.Data
		}
	}
	md, err := RenderMarkdown(values, tpl, m.optionsFor(templateName, datafile))
	if err != nil {
		return "", err
	}

	title := titleFromMarkdown(md)
	if title == "" {
		title = stemOf(datafile)
	}
	if title == "" {
		title = "Untitled"
	}

	body, err := RenderHTML(md)
	if err != nil {
		return "", err
	}

	return composeFullHTML(title, body), nil
}

// titleFromMarkdown returns the frontmatter title, or "" when absent.
func titleFromMarkdown(md string) string {
	fm, _, err := ParseFrontmatter(md)
	if err != nil || fm == nil {
		return ""
	}
	if t, ok := fm["title"].(string); ok {
		return strings.TrimSpace(t)
	}
	return ""
}

// stemOf strips a trailing extension then a `.meta` suffix, so
// "groene-tapenade.meta.json" becomes "groene-tapenade".
func stemOf(name string) string {
	if name == "" {
		return ""
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	return strings.TrimSuffix(name, ".meta")
}

// composeFullHTML wraps the body fragment in a document with the inlined
// stylesheet. Title is HTML-escaped; CSS is trusted (our own embedded
// asset).
func composeFullHTML(title, body string) string {
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n")
	sb.WriteString(`<html lang="en">` + "\n")
	sb.WriteString("<head>\n")
	sb.WriteString(`<meta charset="utf-8">` + "\n")
	sb.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">` + "\n")
	sb.WriteString("<title>")
	sb.WriteString(html.EscapeString(title))
	sb.WriteString("</title>\n")
	sb.WriteString("<style>\n")
	sb.WriteString(proseStylesheet)
	sb.WriteString("\n</style>\n")
	sb.WriteString("</head>\n")
	sb.WriteString(`<body class="formidable-prose">` + "\n")
	sb.WriteString(body)
	sb.WriteString("\n</body>\n")
	sb.WriteString("</html>\n")
	return sb.String()
}
