package codeformatter

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	fmOpenRe  = regexp.MustCompile(`(?m)\A---\s*\n`)
	fmCloseRe = regexp.MustCompile(`(?m)^---\s*$`)
)

// formatMarkdown formats the YAML frontmatter block (if present): first
// applies the structural repair pass (re-nest flat children under their
// schema-known parents), then yaml.v3 canonical re-emit. The markdown
// body is reassembled untouched — Handlebars expressions, code fences,
// and prose all want different rules and getting that wrong is worse
// than doing nothing.
//
// No frontmatter → tidy whitespace pass on the whole source.
// Malformed frontmatter → reassembled with the source frontmatter
// verbatim plus ErrMalformed so the editor can flag it.
func (m *Manager) formatMarkdown(src string) (string, error) {
	open := fmOpenRe.FindStringIndex(src)
	if open == nil {
		return tidy(src), nil
	}
	rest := src[open[1]:]
	closeIdx := fmCloseRe.FindStringIndex(rest)
	if closeIdx == nil {
		return tidy(src), fmt.Errorf("%w: missing closing `---`", ErrMalformed)
	}
	rawYAML := rest[:closeIdx[0]]
	body := rest[closeIdx[1]:]
	body = strings.TrimPrefix(body, "\n")

	repaired := m.repairFlatFrontmatter(rawYAML)
	formatted, ferr := formatYAML(repaired)
	formatted = strings.TrimRight(formatted, "\n")

	out := "---\n" + formatted + "\n---\n"
	if body != "" {
		out += "\n" + body
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if ferr != nil {
		return out, ferr
	}
	return out, nil
}
