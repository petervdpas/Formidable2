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

// BuildFrontmatter prepends a YAML frontmatter block to body; empty data
// returns body unchanged. Sequences are emitted in flow style
// (`tags: [a, b, c]`) rather than yaml.v3's default block style, because
// plugin-export templates author `tags` as flow arrays and block output
// would visibly diverge from what the author wrote. PDF has its own
// BuildFrontmatter and isn't affected.
func BuildFrontmatter(data map[string]any, body string) string {
	if len(data) == 0 {
		return body
	}
	var node yaml.Node
	if err := node.Encode(data); err != nil {
		return body
	}
	setSequenceStyle(&node, yaml.FlowStyle)
	out, err := yaml.Marshal(&node)
	if err != nil {
		return body
	}
	yamlStr := string(out)
	if !strings.HasSuffix(yamlStr, "\n") {
		yamlStr += "\n"
	}
	return "---\n" + yamlStr + "---\n\n" + body
}

// setSequenceStyle applies style to every sequence node, leaving
// mappings and scalars untouched so `title: 'x'` stays quoted as written
// while `tags: [a, b]` lands inline.
func setSequenceStyle(n *yaml.Node, style yaml.Style) {
	if n == nil {
		return
	}
	if n.Kind == yaml.SequenceNode {
		n.Style = style
	}
	for _, c := range n.Content {
		setSequenceStyle(c, style)
	}
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
