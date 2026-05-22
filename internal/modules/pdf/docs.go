package pdf

import (
	"embed"
	"path"
	"strings"
)

// directivesFS embeds the per-locale frontmatter reference documents.
// Each file is `docs/directives.<locale>.md`. English is the fallback.
//
//go:embed docs/directives.*.md
var directivesFS embed.FS

const directivesDir = "docs"
const directivesFallback = "en"

// directivesDoc returns the embedded markdown reference for the given
// locale. Unknown locale falls back to English. The returned string is
// the raw markdown - the caller is responsible for rendering it (the
// Information panel pipes it through render.Service.RenderHTML).
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
