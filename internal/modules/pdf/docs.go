package pdf

import (
	"embed"
	"path"
	"strings"
)

// directivesFS embeds the per-locale frontmatter reference docs
// (docs/directives.<locale>.md); English is the fallback.
//
//go:embed docs/directives.*.md
var directivesFS embed.FS

const directivesDir = "docs"
const directivesFallback = "en"

// directivesDoc returns the raw markdown reference for locale, falling
// back to English. The caller renders it.
func directivesDoc(locale string) (string, error) {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "" {
		locale = directivesFallback
	}
	primary := path.Join(directivesDir, "directives."+locale+".md")
	if b, err := directivesFS.ReadFile(primary); err == nil {
		return string(b), nil
	}
	b, err := directivesFS.ReadFile(path.Join(directivesDir, "directives."+directivesFallback+".md"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
