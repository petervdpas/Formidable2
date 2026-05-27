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

	case "number", "range":
		return asString(val)

	case "multioption", "tags", "list":
		if arr, ok := val.([]any); ok {
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
