package codeformatter

import "strings"

// repairFlatFrontmatter re-indents children that were authored at the
// same column as their schema-known parent. Common pattern:
//
//   cover:
//   title: Hello   ← user forgot the indent
//
// becomes
//
//   cover:
//     title: Hello
//
// Algorithm:
//   - When a line `<key>:` (empty value) at the start of a line names
//     a known parent (per m.schemas), open a block. Track its indent.
//   - Subsequent lines at the SAME indent that are `<key>: <value>`
//     where the key is in the parent's child set get re-indented
//     (+2 spaces from the parent).
//   - Lines that don't match a child key, blank lines, or another
//     known parent close the current block. Lines already indented
//     past the parent (the user got it right) pass through unchanged.
//
// When m.schemas is empty or nil, this is a no-op - yaml.v3 alone
// handles the trivial case.
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

// parseKeyLine extracts the key from a trimmed YAML line of the form
// `key:` or `key: value`. Returns empty key when the line is a comment,
// a list item, or doesn't contain a `:`. hasValue is true when there
// is anything after the `:`.
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
