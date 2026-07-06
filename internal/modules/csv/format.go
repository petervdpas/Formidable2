package csv

import (
	"bytes"
	"encoding/json"
	"strings"
)

// jsonString marshals v the way JavaScript's JSON.stringify does: without
// HTML-escaping &, <, > into & / < / >. Go's json.Marshal
// escapes those by default, which leaked "&" into exported table /
// list cells; the old Formidable produced readable "&".
func jsonString(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimRight(buf.String(), "\n")
}

// FormatValue is the reverse of Coerce - it turns a stored typed value
// back into a CSV-friendly string. Mirrors `formatValue` in the old
// utils/csvTransforms.js.
// indentedRowText returns (text, true) when item is a {text, indent} list row
// (an indented row); export drops the indent to its text. Other shapes (plain
// strings, foreign maps) are left untouched. Kept local so csv stays template-free.
func indentedRowText(item any) (string, bool) {
	if m, ok := item.(map[string]any); ok {
		if s, ok := m["text"].(string); ok {
			return s, true
		}
	}
	return "", false
}

// unindentListItems replaces {text, indent} rows with their text, leaving plain
// strings and any foreign object items as they are.
func unindentListItems(arr []any) []any {
	out := make([]any, len(arr))
	for i, it := range arr {
		if s, ok := indentedRowText(it); ok {
			out[i] = s
		} else {
			out[i] = it
		}
	}
	return out
}

func FormatValue(val any, fieldType string) string {
	if val == nil {
		return ""
	}
	switch fieldType {
	case "boolean":
		if b, ok := val.(bool); ok {
			if b {
				return "true"
			}
			return "false"
		}
		return ""

	case "number", "range", "sequence":
		return asString(val)

	case "multioption", "tags", "list":
		if arr, ok := val.([]any); ok {
			if fieldType == "list" {
				// Export a list unindented: drop indented rows to their text.
				return jsonString(unindentListItems(arr))
			}
			return jsonString(arr)
		}
		return asString(val)

	case "table":
		if arr, ok := val.([]any); ok {
			return jsonString(arr)
		}
		return ""

	default:
		return asString(val)
	}
}
