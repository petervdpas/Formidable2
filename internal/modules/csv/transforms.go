package csv

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

// Mode selects between storage-shaped output (e.g. JSON for split-table)
// and a display-shaped output for preview cells.
type Mode int

const (
	ModeStorage Mode = iota
	ModePreview
)

// rules is the ordered list of supported transform keys, matching the
// dropdown order in the old csvImportModal/csvExportModal.
var rules = []string{
	"none", "lowercase", "uppercase", "capitalize",
	"trim", "trim+lower", "trim+upper", "trim+cap",
	"first-n", "last-n", "split", "bool-match", "split-table",
}

// excludedTypes mirrors utils/csvTransforms.js excludedTypes - field
// types that can never participate in a CSV mapping.
var excludedTypes = []string{"loopstart", "loopstop", "image", "code", "api"}

// Rules returns the canonical ordered rule keys for the UI dropdown.
func Rules() []string {
	out := make([]string, len(rules))
	copy(out, rules)
	return out
}

// ExcludedFieldTypes returns the set of field types that the import/export
// dialogs must skip when listing target fields.
func ExcludedFieldTypes() []string {
	out := make([]string, len(excludedTypes))
	copy(out, excludedTypes)
	return out
}

// Apply runs a transform rule on val. Unknown rules pass val through
// unchanged; bad params (negative N, malformed numbers) likewise no-op.
func Apply(val, rule, param string, mode Mode) string {
	switch rule {
	case "", "none":
		return val
	case "lowercase":
		return strings.ToLower(val)
	case "uppercase":
		return strings.ToUpper(val)
	case "capitalize":
		return titleCase(val)
	case "trim":
		return strings.TrimSpace(val)
	case "trim+lower":
		return strings.ToLower(strings.TrimSpace(val))
	case "trim+upper":
		return strings.ToUpper(strings.TrimSpace(val))
	case "trim+cap":
		return titleCase(strings.TrimSpace(val))
	case "first-n":
		n := parseN(param)
		if n <= 0 || n >= len(val) {
			return val
		}
		return val[:n]
	case "last-n":
		n := parseN(param)
		if n <= 0 || n >= len(val) {
			return val
		}
		return val[len(val)-n:]
	case "split":
		return splitJoin(val, param)
	case "bool-match":
		left := strings.ToLower(strings.TrimSpace(val))
		right := strings.ToLower(strings.TrimSpace(param))
		if left == right && right != "" {
			return "true"
		}
		return "false"
	case "split-table":
		return splitTable(val, param, mode)
	default:
		return val
	}
}

func parseN(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func titleCase(s string) string {
	var b strings.Builder
	inWord := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				b.WriteRune(unicode.ToUpper(r))
				inWord = true
			} else {
				b.WriteRune(r)
			}
		} else {
			b.WriteRune(r)
			inWord = false
		}
	}
	return b.String()
}

func splitJoin(val, sep string) string {
	if sep == "" {
		sep = ","
	}
	parts := strings.Split(val, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

func splitTable(val, param string, mode Mode) string {
	rs, cs := parseSeps(param)
	rawRows := strings.Split(val, rs)
	grid := make([][]string, 0, len(rawRows))
	for _, r := range rawRows {
		cols := strings.Split(r, cs)
		trimmed := make([]string, 0, len(cols))
		anyContent := false
		for _, c := range cols {
			c = strings.TrimSpace(c)
			trimmed = append(trimmed, c)
			if c != "" {
				anyContent = true
			}
		}
		if anyContent {
			grid = append(grid, trimmed)
		}
	}
	if mode == ModePreview {
		rows := make([]string, 0, len(grid))
		for _, r := range grid {
			rows = append(rows, strings.Join(r, ", "))
		}
		return strings.Join(rows, " | ")
	}
	b, _ := json.Marshal(grid)
	return string(b)
}

// parseSeps reads "rowSep colSep" (whitespace-separated). Either piece
// missing falls back to ";" / ",". Mirrors the JS parseSeps helper.
func parseSeps(seps string) (string, string) {
	parts := strings.Fields(seps)
	rs, cs := ";", ","
	if len(parts) > 0 && parts[0] != "" {
		rs = parts[0]
	}
	if len(parts) > 1 && parts[1] != "" {
		cs = parts[1]
	}
	return rs, cs
}
