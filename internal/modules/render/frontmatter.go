package render

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var frontmatterRe = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n*`)

// ParseFrontmatter splits a markdown source into (frontmatter, body).
// When the source has no leading `---\n…\n---` block, frontmatter is
// nil and body is the input verbatim. Invalid YAML is reported via err
// and body falls back to the original source so the caller can still
// render.
func ParseFrontmatter(markdown string) (map[string]any, string, error) {
	if markdown == "" {
		return nil, "", nil
	}
	loc := frontmatterRe.FindStringSubmatchIndex(markdown)
	if loc == nil {
		return nil, markdown, nil
	}
	raw := markdown[loc[2]:loc[3]]
	body := markdown[loc[1]:]

	var data map[string]any
	if err := yaml.Unmarshal([]byte(raw), &data); err != nil {
		return nil, markdown, fmt.Errorf("render: parse frontmatter: %w", err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, body, nil
}

// BuildFrontmatter prepends a YAML frontmatter block to body. An empty
// or nil data map returns body unchanged.
func BuildFrontmatter(data map[string]any, body string) string {
	if len(data) == 0 {
		return body
	}
	out, err := yaml.Marshal(data)
	if err != nil {
		return body
	}
	yamlStr := string(out)
	if !strings.HasSuffix(yamlStr, "\n") {
		yamlStr += "\n"
	}
	return "---\n" + yamlStr + "---\n\n" + body
}

// FilterFrontmatter returns a copy of data containing only the named
// keys (in any order). Missing keys are silently skipped.
func FilterFrontmatter(data map[string]any, keep []string) map[string]any {
	out := map[string]any{}
	if len(keep) == 0 {
		return out
	}
	for _, k := range keep {
		if v, ok := data[k]; ok {
			out[k] = v
		}
	}
	return out
}
