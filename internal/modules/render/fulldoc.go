package render

import (
	_ "embed"
	"fmt"
	"html"
	"strings"
)

// proseStylesheet — single-source-of-truth wiki-prose CSS, embedded
// at build time so RenderFullHTML can produce a self-contained doc
// without filesystem lookups. The same file is imported by Vite into
// the SPA bundle (see frontend/src/styles/index.css), keeping the
// preview slideout and exported HTML pixel-identical.
//
//go:embed assets/formidable-prose.css
var proseStylesheet string

// RenderFullHTML returns a self-contained HTML document with:
//   - DOCTYPE + html/head/body scaffolding
//   - <title> sourced from the markdown's frontmatter "title:" key,
//     falling back to the datafile stem (or "Untitled" if datafile is
//     empty)
//   - inlined formidable-prose stylesheet so the doc renders the same
//     way it does in the in-app preview
//   - the rendered fragment as the body, wrapped in
//     <body class="formidable-prose">
//
// Reused by the storage workspace's "Copy HTML" action and the future
// internal wiki HTTP server.
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
	md, err := RenderMarkdown(values, tpl, m.optionsFor(templateName))
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

// titleFromMarkdown returns the frontmatter title (string-typed) or
// "" when there's no frontmatter or no title key.
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

// stemOf strips a single trailing extension, then strips a `.meta`
// suffix (so "groene-tapenade.meta.json" → "groene-tapenade"). Empty
// input → empty result.
func stemOf(name string) string {
	if name == "" {
		return ""
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	return strings.TrimSuffix(name, ".meta")
}

// composeFullHTML wraps the rendered body fragment in a document with
// the inlined stylesheet. Title is HTML-escaped; CSS is treated as
// trusted (it's our own embedded asset).
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
