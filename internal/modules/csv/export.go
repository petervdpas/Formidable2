package csv

import (
	"encoding/json"
	"strings"
)

// Transform pairs a transform rule key with its rule-specific param.
type Transform struct {
	Rule  string `json:"rule"`
	Param string `json:"param"`
}

// ExportColumn describes one CSV column. SourceKeys may carry a single
// field key (plain export), several keys (computed/concat column), or
// a dotted "field.subkey" form used to project a table column into the
// row when alignment is enabled.
type ExportColumn struct {
	Header     string    `json:"header"`
	SourceKeys []string  `json:"sourceKeys"`
	Separator  string    `json:"separator"`
	Transform  Transform `json:"transform"`
}

// ExportPlan is the full request the dialog sends to BuildExportRows.
// AlignSource is optional: when set to a list- or table-typed field
// key, each entry unrolls into one CSV row per item in that field.
type ExportPlan struct {
	Columns     []ExportColumn `json:"columns"`
	AlignSource string         `json:"alignSource"`
}

// ExportResult is what the dialog gets back. Rows includes the header
// row at index 0; Count is the number of source entries exported
// (not row count, since alignment can multiply rows).
type ExportResult struct {
	Rows  [][]string `json:"rows"`
	Count int        `json:"count"`
	Error string     `json:"error,omitempty"`
}

// Export loads every form for the named template through the forms
// dependency and runs BuildExportRows on them. Returns ExportResult
// with Error set when the storage dep was never wired or listing fails.
func (m *Manager) Export(templateFilename string, plan ExportPlan, fields []FieldSpec) ExportResult {
	if m.forms == nil {
		return ExportResult{Error: "csv: storage dependency not configured"}
	}
	files, err := m.forms.ListForms(templateFilename)
	if err != nil {
		return ExportResult{Error: err.Error()}
	}
	entries := make([]map[string]any, 0, len(files))
	for _, fn := range files {
		data := m.forms.LoadFormData(templateFilename, fn)
		if data == nil {
			continue
		}
		entries = append(entries, data)
	}
	return ExportResult{
		Rows:  BuildExportRows(plan, entries, fields),
		Count: len(entries),
	}
}

// BuildExportRows turns a stream of stored entries into the rectangle
// of strings that csv.Manager.Write expects. The first row is the
// header. Mirrors handleCsvExport's row-building loop.
func BuildExportRows(plan ExportPlan, entries []map[string]any, fields []FieldSpec) [][]string {
	fieldByKey := make(map[string]FieldSpec, len(fields))
	for _, f := range fields {
		fieldByKey[f.Key] = f
	}

	alignField, alignEnabled := "", false
	if plan.AlignSource != "" {
		if f, ok := fieldByKey[plan.AlignSource]; ok && (f.Type == "list" || f.Type == "table") {
			alignField, alignEnabled = plan.AlignSource, true
		}
	}

	header := make([]string, len(plan.Columns))
	for i, c := range plan.Columns {
		header[i] = c.Header
	}
	rows := [][]string{header}

	for _, entry := range entries {
		if !alignEnabled {
			row := make([]string, len(plan.Columns))
			for j, col := range plan.Columns {
				row[j] = buildExportCell(entry, col, fieldByKey, "", -1)
			}
			rows = append(rows, row)
			continue
		}

		arr, _ := entry[alignField].([]any)
		n := len(arr)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			row := make([]string, len(plan.Columns))
			for j, col := range plan.Columns {
				row[j] = buildExportCell(entry, col, fieldByKey, alignField, i)
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func buildExportCell(entry map[string]any, col ExportColumn, fields map[string]FieldSpec, alignSource string, alignIdx int) string {
	if len(col.SourceKeys) == 0 {
		return ""
	}
	sep := col.Separator
	if sep == "" {
		sep = " "
	}
	parts := make([]string, 0, len(col.SourceKeys))
	for _, key := range col.SourceKeys {
		parts = append(parts, resolveSourceValue(entry, key, fields, alignSource, alignIdx))
	}
	joined := strings.Join(parts, sep)
	if col.Transform.Rule == "" || col.Transform.Rule == "none" {
		return joined
	}
	return Apply(joined, col.Transform.Rule, col.Transform.Param, ModeStorage)
}

func resolveSourceValue(entry map[string]any, sourceKey string, fields map[string]FieldSpec, alignSource string, alignIdx int) string {
	if sourceKey == "" {
		return ""
	}
	root, sub := splitDotted(sourceKey)
	rootField, hasField := fields[root]
	if !hasField {
		return ""
	}

	if alignSource != "" && root == alignSource && alignIdx >= 0 {
		arr, ok := entry[root].([]any)
		if !ok || alignIdx >= len(arr) {
			return ""
		}
		item := arr[alignIdx]
		if item == nil {
			return ""
		}
		if sub != "" {
			// Table cell: positional array - resolve sub → column index
			// via the field's option list.
			if cells, isArr := item.([]any); isArr {
				idx := findOptionIndex(rootField.Options, sub)
				if idx < 0 || idx >= len(cells) {
					return ""
				}
				return asString(cells[idx])
			}
			// Object-shaped item (legacy/foreign data) - index by key.
			if m, isMap := item.(map[string]any); isMap {
				return asString(m[sub])
			}
			return ""
		}
		// Bare alignment root - stringify the item.
		if m, isMap := item.(map[string]any); isMap {
			b, _ := json.Marshal(m)
			return string(b)
		}
		return asString(item)
	}

	return FormatValue(entry[root], rootField.Type)
}

func splitDotted(s string) (string, string) {
	for i, r := range s {
		if r == '.' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func findOptionIndex(options []any, value string) int {
	for i, o := range options {
		v, _ := optionPair(o)
		if v == value {
			return i
		}
	}
	return -1
}

