package csv

import "encoding/json"

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
			b, _ := json.Marshal(arr)
			return string(b)
		}
		return asString(val)

	case "table":
		if arr, ok := val.([]any); ok {
			b, _ := json.Marshal(arr)
			return string(b)
		}
		return ""

	default:
		return asString(val)
	}
}
