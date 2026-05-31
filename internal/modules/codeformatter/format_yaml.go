package codeformatter

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// formatYAML round-trips src through yaml.v3 to produce canonical
// 2-space-indented output with key order preserved. Malformed YAML returns a
// tidy-cleaned copy plus ErrMalformed so the caller surfaces the parse error
// without losing the user's text.
//
// yaml.v3 emits what it parsed: a flat input stays flat. This is a YAML
// formatter, not a schema repairer; PDF-domain re-nesting belongs in
// pdf.MigrateFrontmatter.
func formatYAML(src string) (string, error) {
	if strings.TrimSpace(src) == "" {
		return src, nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(src), &node); err != nil {
		return tidy(src), fmt.Errorf("%w: yaml: %v", ErrMalformed, err)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return tidy(src), fmt.Errorf("%w: yaml encode: %v", ErrMalformed, err)
	}
	_ = enc.Close()
	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}
