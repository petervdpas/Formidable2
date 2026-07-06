package template

import "fmt"

// A list field's value is a flat array whose items are usually plain strings.
// A row the author indents is stored instead as {text, indent} (indent > 0), so
// a list with no indentation stays a pure []string and nothing downstream breaks.
// These helpers are the one place that knows both shapes; render, sort and dedup
// read items through them.

// MaxListIndent caps the indent level so a hostile or runaway value can't emit a
// pathologically deep list.
const MaxListIndent = 8

// ListItemText returns a list item's text: the string itself, the "text" of a
// {text, indent} object, or the scalar stringified (numbers/bools, as before).
func ListItemText(item any) string {
	switch t := item.(type) {
	case nil:
		return ""
	case string:
		return t
	case map[string]any:
		if s, ok := t["text"].(string); ok {
			return s
		}
		return ""
	default:
		return fmt.Sprint(t)
	}
}

// ListItemIndent returns a list item's indent level: 0 for a plain string, else
// the object's "indent", clamped to [0, MaxListIndent].
func ListItemIndent(item any) int {
	m, ok := item.(map[string]any)
	if !ok {
		return 0
	}
	n := 0
	switch v := m["indent"].(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	case int64:
		n = int(v)
	}
	if n < 0 {
		return 0
	}
	if n > MaxListIndent {
		return MaxListIndent
	}
	return n
}
