package index

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// dateLayouts mirrors the formats render/dateFormat recognises, so the
// index parses exactly what the renderer treats as a date. The first
// match wins; "2006-01-02" is Formidable's `date` field storage.
var dateLayouts = []string{
	"2006-01-02",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

// pickValues projects a (template fields, form data) pair into the
// aggregatable rows stored in form_values. Only field types that make
// chart sense are materialised: numeric (number, range), date,
// boolean, single-choice (dropdown, radio), multi-choice (multioption,
// one row per selection), and table columns (one row per cell, typed
// from the column's declared `type`). Free text, tags (own table),
// guid, api and image fields are skipped so the index stays lean.
func pickValues(fields []template.Field, data map[string]any) []FormValueRow {
	var out []FormValueRow
	for _, fld := range fields {
		raw, ok := data[fld.Key]
		if !ok {
			continue
		}
		switch fld.Type {
		case "number", "range":
			out = append(out, scalarRow(fld.Key, "number", raw))
		case "date":
			if r, ok := dateRow(fld.Key, nil, raw); ok {
				out = append(out, r)
			}
		case "boolean":
			out = append(out, boolRow(fld.Key, raw))
		case "dropdown", "radio":
			if s := asText(raw); s != "" {
				out = append(out, FormValueRow{FieldKey: fld.Key, ValueType: "text", Text: s})
			}
		case "multioption":
			for _, item := range asSlice(raw) {
				if s := asText(item); s != "" {
					out = append(out, FormValueRow{FieldKey: fld.Key, ValueType: "text", Text: s})
				}
			}
		case "table":
			out = append(out, tableRows(fld, raw)...)
		}
	}
	return out
}

// scalarRow builds a numeric scalar row. Num is nil when raw doesn't
// parse as a number, but the row is still emitted so "count present"
// and distribution-over-text stay possible.
func scalarRow(key, valueType string, raw any) FormValueRow {
	r := FormValueRow{FieldKey: key, ValueType: valueType, Text: asText(raw)}
	if n, ok := asNumber(raw); ok {
		r.Num = &n
	}
	return r
}

// dateRow parses raw against the recognised layouts. Returns ok=false
// when nothing parses, so the caller can skip it (an unparseable date
// is noise, not a data point). col is carried through for table cells.
func dateRow(key string, col *int, raw any) (FormValueRow, bool) {
	s := strings.TrimSpace(asText(raw))
	if s == "" {
		return FormValueRow{}, false
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			epoch := float64(t.UTC().Unix())
			return FormValueRow{
				FieldKey:  key,
				Col:       col,
				ValueType: "date",
				Num:       &epoch,
				Text:      t.UTC().Format("2006-01-02"),
			}, true
		}
	}
	return FormValueRow{}, false
}

// boolRow stores both representations: Text "true"/"false" for
// distribution, Num 1/0 for sum/avg ("how many done").
func boolRow(key string, raw any) FormValueRow {
	b := truthy(raw)
	n := 0.0
	text := "false"
	if b {
		n = 1
		text = "true"
	}
	return FormValueRow{FieldKey: key, ValueType: "bool", Num: &n, Text: text}
}

// tableRows fans a table field's matrix into per-cell rows. Each cell's
// type comes from the column definition at its index (field.Options[i]
// is a {value, type, label} map); columns with no definition default to
// text. Date cells that don't parse are dropped, matching scalar dates.
func tableRows(fld template.Field, raw any) []FormValueRow {
	matrix := asSlice(raw)
	if len(matrix) == 0 {
		return nil
	}
	colTypes := tableColumnTypes(fld.Options)
	var out []FormValueRow
	for _, rowAny := range matrix {
		cells := asSlice(rowAny)
		for i, cell := range cells {
			col := i
			colType := "string"
			if i < len(colTypes) {
				colType = colTypes[i]
			}
			switch colType {
			case "number":
				r := scalarRow(fld.Key, "number", cell)
				r.Col = &col
				out = append(out, r)
			case "date":
				if r, ok := dateRow(fld.Key, &col, cell); ok {
					out = append(out, r)
				}
			default:
				if s := asText(cell); s != "" {
					c := col
					out = append(out, FormValueRow{FieldKey: fld.Key, Col: &c, ValueType: "text", Text: s})
				}
			}
		}
	}
	return out
}

// tableColumnTypes extracts the declared per-column type list from a
// table field's Options. Each option is a {value, type, label} map;
// missing/blank type falls back to "string".
func tableColumnTypes(options []any) []string {
	out := make([]string, 0, len(options))
	for _, opt := range options {
		m, ok := opt.(map[string]any)
		if !ok {
			out = append(out, "string")
			continue
		}
		t, _ := m["type"].(string)
		if t == "" {
			t = "string"
		}
		out = append(out, t)
	}
	return out
}

// asNumber coerces JSON numbers and numeric strings to float64.
func asNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// asText stringifies a scalar value for the text_value column.
func asText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}

// asSlice returns v as []any, or nil when it isn't a slice.
func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

// truthy mirrors the boolean coercion used elsewhere: real bools pass
// through; strings "true"/"1"/"yes"/"on" are true; numbers are true
// when non-zero.
func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes", "on":
			return true
		}
		return false
	case float64:
		return x != 0
	}
	return false
}
