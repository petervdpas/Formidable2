// Package codeformatter is the backend code-format pass the template
// editor calls instead of running prettier in the webview. Standalone:
// no template/pdf/render imports. The schemas registry is supplied at
// construction time (composition root wires pdf.Schemas() in), so
// codeformatter knows nothing about picoloom keys - it only knows how
// to apply a "parent → children" hint set to flat YAML.
package codeformatter

import (
	"errors"
	"strings"
)

type Lang string

const (
	LangMarkdown Lang = "markdown"
	LangYAML     Lang = "yaml"
	LangLua      Lang = "lua"
)

// ErrMalformed wraps any parser failure surfaced by a sub-formatter.
// errors.Is(err, ErrMalformed) distinguishes "the source is broken,
// leave it alone" from infrastructure errors.
var ErrMalformed = errors.New("codeformatter: source malformed")

// Schemas maps a top-level frontmatter key (the "parent") to the set
// of child key names that belong under it. Used by the markdown
// formatter's repair pass to re-nest children that were authored at
// the same column as their parent.
type Schemas map[string]map[string]bool

type Manager struct {
	schemas Schemas
}

// NewManager builds a Manager with the given schema registry. Pass
// nil for a generic formatter with no repair pass (yaml round-trip
// only).
func NewManager(s Schemas) *Manager { return &Manager{schemas: s} }

// Format reformats src for the named language. Returned string is the
// cleaned source; err is non-nil only when parsing failed - in that
// case the returned string is still safe to display (whitespace tidy).
// Unknown lang values fall through to tidy.
func (m *Manager) Format(lang, src string) (string, error) {
	switch Lang(lang) {
	case LangMarkdown:
		return m.formatMarkdown(src)
	case LangYAML:
		return formatYAML(src)
	case LangLua:
		return formatLua(src)
	}
	return tidy(src), nil
}

// tidy: trim trailing whitespace, normalize line endings to \n, collapse
// runs of >2 blank lines, ensure exactly one trailing newline. Safe for
// any text - never alters indentation or content.
func tidy(src string) string {
	out := strings.ReplaceAll(src, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	out = strings.Join(lines, "\n")
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	out = strings.TrimRight(out, "\n") + "\n"
	return out
}
