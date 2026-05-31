package codeformatter

import "strings"

// repairFlatFrontmatter re-indents children authored at the same column as
// their schema-known parent (per m.schemas):
//
//	cover:            cover:
//	title: Hello  ->    title: Hello
//
// No-op when m.schemas is empty.
func (m *Manager) repairFlatFrontmatter(src string) string {
	if len(m.schemas) == 0 {
		return src
	}
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))

	var parent string
	var children map[string]bool
	parentIndent := -1

	closeBlock := func() {
		parent = ""
		children = nil
		parentIndent = -1
	}

	for _, line := range lines {
		indent := countLeadingSpaces(line)
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			closeBlock()
			out = append(out, line)
			continue
		}

		key, hasValue := parseKeyLine(trimmed)
		if key != "" && !hasValue {
			if childSet, ok := m.schemas[key]; ok && indent == 0 {
				parent = key
				children = childSet
				parentIndent = indent
				out = append(out, line)
				continue
			}
			closeBlock()
			out = append(out, line)
			continue
		}

		if parent != "" && key != "" && indent == parentIndent && children[key] {
			out = append(out, strings.Repeat(" ", parentIndent+2)+trimmed)
			continue
		}

		if parent != "" && indent > parentIndent {
			out = append(out, line)
			continue
		}

		closeBlock()
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// parseKeyLine extracts the key from a trimmed `key:` or `key: value` line.
// Empty key for comments, list items, or no colon; hasValue if anything
// follows the colon.
func parseKeyLine(trimmed string) (key string, hasValue bool) {
	if trimmed == "" || trimmed[0] == '#' || trimmed[0] == '-' {
		return "", false
	}
	idx := strings.Index(trimmed, ":")
	if idx <= 0 {
		return "", false
	}
	k := trimmed[:idx]
	for i := 0; i < len(k); i++ {
		r := k[i]
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-') {
			return "", false
		}
	}
	rest := strings.TrimSpace(trimmed[idx+1:])
	return k, rest != ""
}

func countLeadingSpaces(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			break
		}
		n++
	}
	return n
}
