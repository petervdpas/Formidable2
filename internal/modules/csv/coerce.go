package csv

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Coerce turns a raw CSV cell string into the typed Go value appropriate
// for the named template field type. Mirrors `coerceValue` in the old
// utils/csvTransforms.js. `options` carries the field's option list
// (only meaningful for dropdown/radio/multioption).
func Coerce(raw, fieldType string, options []any) any {
	val := strings.TrimSpace(raw)
	switch fieldType {
	case "boolean":
		switch strings.ToLower(val) {
		case "true", "1", "yes", "on":
			return true
		}
		return false

	case "number":
		if n, err := strconv.ParseFloat(val, 64); err == nil {
			return n
		}
		return float64(0)

	case "range":
		if n, err := strconv.ParseFloat(val, 64); err == nil {
			return n
		}
		return float64(50)

	case "date":
		return val

	case "dropdown", "radio":
		return matchOption(val, options)

	case "multioption":
		items := parseAsList(val)
		out := make([]any, 0, len(items))
		for _, v := range items {
			out = append(out, matchOption(asString(v), options))
		}
		return out

	case "tags", "list":
		return parseAsList(val)

	case "table":
		if val == "" {
			return []any{}
		}
		var parsed any
		if err := json.Unmarshal([]byte(val), &parsed); err == nil {
			if arr, ok := parsed.([]any); ok {
				return arr
			}
		}
		return []any{}

	default:
		return val
	}
}

// CoercePreview returns a display-friendly string for the typed value
// Coerce would produce. Mirrors `previewCoerce` in the JS source.
func CoercePreview(raw, fieldType string, options []any) string {
	val := strings.TrimSpace(raw)
	switch fieldType {
	case "boolean":
		switch strings.ToLower(val) {
		case "true", "1", "yes", "on":
			return "true"
		}
		return "false"

	case "number":
		if _, err := strconv.ParseFloat(val, 64); err == nil {
			return val
		}
		return "0"

	case "range":
		if _, err := strconv.ParseFloat(val, 64); err == nil {
			return val
		}
		return "50"

	case "dropdown", "radio":
		return asString(matchOption(val, options))

	case "multioption":
		items := parseAsList(val)
		parts := make([]string, 0, len(items))
		for _, v := range items {
			parts = append(parts, asString(matchOption(asString(v), options)))
		}
		return strings.Join(parts, ", ")

	case "tags", "list":
		items := parseAsList(val)
		parts := make([]string, 0, len(items))
		for _, v := range items {
			parts = append(parts, asString(v))
		}
		return strings.Join(parts, ", ")

	default:
		return val
	}
}

// matchOption resolves `raw` against an option list. Options may be
// `{value, label}` maps or bare scalar strings. Match is case-insensitive,
// value-first then label. Unmatched values pass through.
func matchOption(raw string, options []any) any {
	if len(options) == 0 || raw == "" {
		return raw
	}
	lower := strings.ToLower(raw)

	// First pass: value match
	for _, o := range options {
		val, _ := optionPair(o)
		if strings.ToLower(val) == lower {
			return val
		}
	}
	// Second pass: label match
	for _, o := range options {
		val, label := optionPair(o)
		if label != "" && strings.ToLower(label) == lower {
			return val
		}
	}
	return raw
}

// optionPair extracts {value, label} from one option entry, accepting
// either a {value, label} map or a bare scalar (used as both).
func optionPair(o any) (string, string) {
	switch v := o.(type) {
	case map[string]any:
		return asString(v["value"]), asString(v["label"])
	case map[any]any:
		return asString(v["value"]), asString(v["label"])
	default:
		s := asString(v)
		return s, s
	}
}

// parseAsList parses a string as a list. JSON array first; otherwise
// split on `[,;|]` with trim+filter-empty. Mirrors the JS helper.
func parseAsList(val string) []any {
	if val == "" {
		return []any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(val), &parsed); err == nil {
		if arr, ok := parsed.([]any); ok {
			return arr
		}
	}
	out := []any{}
	for _, p := range splitAny(val, ",;|") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitAny splits s on any rune in seps.
func splitAny(s, seps string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(seps, r)
	})
}

// asString stringifies a JSON-shaped scalar without quoting it.
func asString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		// JSON numbers come back as float64; keep an int-looking number tidy.
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", x)
	}
}
