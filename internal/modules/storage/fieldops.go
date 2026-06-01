package storage

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// SortFieldValue fetches a list/table field from the saved record (the pointer
// is template + datafile + fieldKey), sorts it, and returns the sorted value.
// It reads disk but never writes: persistence stays with the normal save path,
// and the value is fetched from the file, not pushed from frontend memory. For
// table fields, column is the column key to sort by (empty = first column);
// direction is "asc" (default) or "desc".
func (m *Manager) SortFieldValue(templateFilename, datafile, fieldKey, column, direction string) (any, error) {
	field, value, err := m.fetchField(templateFilename, datafile, fieldKey)
	if err != nil {
		return nil, err
	}
	desc := strings.EqualFold(direction, "desc")
	switch field.Type {
	case "list":
		return sortList(asSlice(value), desc), nil
	case "table":
		idx, colType, err := resolveColumn(field, column)
		if err != nil {
			return nil, err
		}
		return sortTable(asSlice(value), idx, colType, desc), nil
	default:
		return nil, fmt.Errorf("field %q (type %q) is not sortable", fieldKey, field.Type)
	}
}

// DedupFieldValue fetches a list/table field from the saved record, removes
// duplicates (keeping the first occurrence and original order), and returns the
// result. Same read-only, pointer-driven contract as SortFieldValue. For table
// fields, column is the key whose value identifies a duplicate row (empty =
// first column).
func (m *Manager) DedupFieldValue(templateFilename, datafile, fieldKey, column string) (any, error) {
	field, value, err := m.fetchField(templateFilename, datafile, fieldKey)
	if err != nil {
		return nil, err
	}
	switch field.Type {
	case "list":
		return dedupList(asSlice(value)), nil
	case "table":
		idx, _, err := resolveColumn(field, column)
		if err != nil {
			return nil, err
		}
		return dedupTable(asSlice(value), idx), nil
	default:
		return nil, fmt.Errorf("field %q (type %q) cannot be deduplicated", fieldKey, field.Type)
	}
}

// fetchField resolves the template field plus its current value from the saved
// record on disk. A missing record or unknown field is an error.
func (m *Manager) fetchField(templateFilename, datafile, fieldKey string) (template.Field, any, error) {
	field, ok := m.findField(templateFilename, fieldKey)
	if !ok {
		return template.Field{}, nil, fmt.Errorf("unknown field %q", fieldKey)
	}
	form := m.LoadForm(templateFilename, datafile)
	if form == nil {
		return template.Field{}, nil, fmt.Errorf("record %q not found (save it first)", datafile)
	}
	return field, form.Data[fieldKey], nil
}

// findField returns the template field with the given key.
func (m *Manager) findField(templateFilename, fieldKey string) (template.Field, bool) {
	for _, f := range m.fieldsFor(templateFilename) {
		if f.Key == fieldKey {
			return f, true
		}
	}
	return template.Field{}, false
}

// resolveColumn maps a table column key to its index + declared type. An empty
// key selects the first column; an unknown non-empty key is an error.
func resolveColumn(f template.Field, column string) (idx int, colType string, err error) {
	if len(f.Options) == 0 {
		return 0, "", fmt.Errorf("table field %q has no columns", f.Key)
	}
	if column == "" {
		return 0, columnType(f.Options[0]), nil
	}
	for i, opt := range f.Options {
		if columnKey(opt) == column {
			return i, columnType(opt), nil
		}
	}
	return 0, "", fmt.Errorf("column %q not found in field %q", column, f.Key)
}

func columnKey(opt any) string {
	if m, ok := opt.(map[string]any); ok {
		if s, ok := m["value"].(string); ok {
			return s
		}
	}
	return ""
}

func columnType(opt any) string {
	if m, ok := opt.(map[string]any); ok {
		if s, ok := m["type"].(string); ok && s != "" {
			return s
		}
	}
	return "string"
}

// asSlice coerces a stored field value to []any. List/table values load from
// JSON as []any; a legacy "" or absent value yields an empty slice.
func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return []any{}
}

// cellAt returns the cell at idx in a table row, or nil when the row is short.
func cellAt(row any, idx int) any {
	cells, ok := row.([]any)
	if !ok || idx < 0 || idx >= len(cells) {
		return nil
	}
	return cells[idx]
}

// sortList returns a stable, natural-ordered copy of a list field's items.
func sortList(items []any, desc bool) []any {
	out := append([]any(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		c := naturalCompare(toStr(out[i]), toStr(out[j]))
		if desc {
			return c > 0
		}
		return c < 0
	})
	return out
}

// sortTable returns a stable copy of a table field's rows ordered by one column,
// using a comparator matched to that column's declared type.
func sortTable(rows []any, colIdx int, colType string, desc bool) []any {
	out := append([]any(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		c := compareCells(cellAt(out[i], colIdx), cellAt(out[j], colIdx), colType)
		if desc {
			return c > 0
		}
		return c < 0
	})
	return out
}

// dedupList drops repeated items, keeping the first occurrence and original order.
func dedupList(items []any) []any {
	seen := make(map[string]struct{}, len(items))
	out := make([]any, 0, len(items))
	for _, it := range items {
		k := toStr(it)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, it)
	}
	return out
}

// dedupTable drops rows whose key column repeats an earlier row, keeping the
// first occurrence and original order.
func dedupTable(rows []any, colIdx int) []any {
	seen := make(map[string]struct{}, len(rows))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		k := toStr(cellAt(r, colIdx))
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, r)
	}
	return out
}

// compareCells orders two table cells by the column's declared type, falling
// back to a natural string compare when a typed parse fails.
func compareCells(a, b any, colType string) int {
	switch colType {
	case "number":
		fa, oka := toFloat(a)
		fb, okb := toFloat(b)
		if oka && okb {
			switch {
			case fa < fb:
				return -1
			case fa > fb:
				return 1
			default:
				return 0
			}
		}
	case "bool":
		ba, bb := toBool(a), toBool(b)
		switch {
		case ba == bb:
			return 0
		case !ba:
			return -1
		default:
			return 1
		}
	case "date":
		ta, oka := toTime(a)
		tb, okb := toTime(b)
		if oka && okb {
			switch {
			case ta.Before(tb):
				return -1
			case ta.After(tb):
				return 1
			default:
				return 0
			}
		}
	}
	return naturalCompare(toStr(a), toStr(b))
}

// naturalCompare orders two strings case-insensitively with embedded digit runs
// compared numerically (so "x2" sorts before "x10"). Ties break by raw bytes
// to stay deterministic.
func naturalCompare(a, b string) int {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(la) && j < len(lb) {
		ca, cb := la[i], lb[j]
		if isDigit(ca) && isDigit(cb) {
			si, sj := i, j
			for i < len(la) && isDigit(la[i]) {
				i++
			}
			for j < len(lb) && isDigit(lb[j]) {
				j++
			}
			na := strings.TrimLeft(la[si:i], "0")
			nb := strings.TrimLeft(lb[sj:j], "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			continue
		}
		if ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case len(la)-i < len(lb)-j:
		return -1
	case len(la)-i > len(lb)-j:
		return 1
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func toStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	default:
		return ""
	}
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	case float64:
		return t != 0
	default:
		return false
	}
}

var dateLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02", "2006/01/02", "02-01-2006"}

func toTime(v any) (time.Time, bool) {
	s := strings.TrimSpace(toStr(v))
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
