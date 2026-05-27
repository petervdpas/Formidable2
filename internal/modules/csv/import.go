package csv

import (
	"strconv"
	"strings"
)

// ImportColumn maps one CSV header onto an import target. Target is a
// plain field key ("audit_control_id"), or a dotted "tablekey.subkey"
// addressing one column of the aligned table (the inverse of the dotted
// source keys the export writes).
type ImportColumn struct {
	Header    string    `json:"header"`
	Target    string    `json:"target"`
	Transform Transform `json:"transform"`
}

// ImportPlan describes how CSV rows reconstruct form entries, the inverse
// of ExportPlan. When AlignSource is empty every row becomes one entry.
// When AlignSource names a list/table field, rows sharing the same
// GroupKey value collapse into a single entry whose aligned field gathers
// one item per row, undoing the export's row-multiplication.
type ImportPlan struct {
	Columns     []ImportColumn `json:"columns"`
	AlignSource string         `json:"alignSource"`
	GroupKey    string         `json:"groupKey"`
}

// ImportForm is one reconstructed entry. Key is the group key value that
// identified it (empty when ungrouped); callers use it to derive a stable
// filename.
type ImportForm struct {
	Key  string         `json:"key"`
	Data map[string]any `json:"data"`
}

// BuildImportForms turns parsed CSV rows into reconstructed entries per the
// plan. It is the inverse of BuildExportRows: an unaligned plan yields one
// entry per row, an aligned plan groups rows back into entries with their
// nested list/table rebuilt.
func BuildImportForms(plan ImportPlan, headers []string, rows [][]string, fields []FieldSpec) []ImportForm {
	fieldByKey := make(map[string]FieldSpec, len(fields))
	for _, f := range fields {
		fieldByKey[f.Key] = f
	}
	headerIdx := make(map[string]int, len(headers))
	for i, h := range headers {
		headerIdx[h] = i
	}

	cell := func(row []string, col ImportColumn) string {
		v := ""
		if idx, ok := headerIdx[col.Header]; ok && idx < len(row) {
			v = row[idx]
		}
		if col.Transform.Rule != "" && col.Transform.Rule != "none" {
			v = Apply(v, col.Transform.Rule, col.Transform.Param, ModeStorage)
		}
		return v
	}

	alignKey, alignType := "", ""
	if plan.AlignSource != "" {
		if f, ok := fieldByKey[plan.AlignSource]; ok && (f.Type == "list" || f.Type == "table") {
			alignKey, alignType = f.Key, f.Type
		}
	}

	if alignKey == "" {
		out := make([]ImportForm, 0, len(rows))
		for _, row := range rows {
			data := map[string]any{}
			applyFlatColumns(data, plan.Columns, row, cell, fieldByKey, "")
			out = append(out, ImportForm{Data: data})
		}
		return out
	}

	// Does any column carry the group key? Without one, grouping can't
	// merge rows, so each row becomes its own (single-item) entry.
	hasGroupCol := false
	for _, c := range plan.Columns {
		if c.Target == plan.GroupKey && plan.GroupKey != "" {
			hasGroupCol = true
			break
		}
	}

	order := []string{}
	grouped := map[string][][]string{}
	for i, row := range rows {
		gk := ""
		if hasGroupCol {
			for _, c := range plan.Columns {
				if c.Target == plan.GroupKey {
					gk = cell(row, c)
					break
				}
			}
		}
		if !hasGroupCol || gk == "" {
			gk = "\x00row\x00" + strconv.Itoa(i) // unique per row → no merge
		}
		if _, ok := grouped[gk]; !ok {
			order = append(order, gk)
		}
		grouped[gk] = append(grouped[gk], row)
	}

	alignField := fieldByKey[alignKey]
	out := make([]ImportForm, 0, len(order))
	for _, gk := range order {
		groupRows := grouped[gk]
		data := map[string]any{}
		applyFlatColumns(data, plan.Columns, groupRows[0], cell, fieldByKey, alignKey)
		items := make([]any, 0, len(groupRows))
		for _, row := range groupRows {
			if alignType == "table" {
				items = append(items, tableRowCells(alignField, plan.Columns, row, cell))
			} else {
				items = append(items, listItem(alignKey, plan.Columns, row, cell))
			}
		}
		data[alignKey] = items
		key := gk
		if strings.HasPrefix(key, "\x00row\x00") {
			key = ""
		}
		out = append(out, ImportForm{Key: key, Data: data})
	}
	return out
}

// applyFlatColumns sets top-level fields from columns whose target is a
// plain field key (not dotted, not the aligned field). Several columns
// pointing at one field join with a space before coercion, mirroring the
// export's concat columns. skipKey drops the aligned field, which is
// rebuilt separately.
func applyFlatColumns(
	data map[string]any,
	cols []ImportColumn,
	row []string,
	cell func([]string, ImportColumn) string,
	fieldByKey map[string]FieldSpec,
	skipKey string,
) {
	parts := map[string][]string{}
	order := []string{}
	for _, c := range cols {
		root, sub := splitDotted(c.Target)
		if sub != "" || root == "" || root == skipKey {
			continue
		}
		if _, ok := fieldByKey[root]; !ok {
			continue
		}
		if _, seen := parts[root]; !seen {
			order = append(order, root)
		}
		parts[root] = append(parts[root], cell(row, c))
	}
	for _, k := range order {
		f := fieldByKey[k]
		data[k] = Coerce(strings.Join(parts[k], " "), f.Type, f.Options)
	}
}

// tableRowCells builds one positional cell row for the aligned table from
// the dotted "tablekey.subkey" columns, mapping each subkey to its column
// index via the field's option list and coercing by column type. Unmapped
// columns are left empty.
func tableRowCells(field FieldSpec, cols []ImportColumn, row []string, cell func([]string, ImportColumn) string) []any {
	width := len(field.Options)
	cells := make([]any, width)
	for i := range cells {
		cells[i] = ""
	}
	for _, c := range cols {
		root, sub := splitDotted(c.Target)
		if root != field.Key || sub == "" {
			continue
		}
		idx := findOptionIndex(field.Options, sub)
		if idx < 0 || idx >= width {
			continue
		}
		cells[idx] = coerceCell(field.Options[idx], cell(row, c))
	}
	return cells
}

// listItem pulls one list entry from the column targeting the bare align
// root. List items are plain strings.
func listItem(alignKey string, cols []ImportColumn, row []string, cell func([]string, ImportColumn) string) any {
	for _, c := range cols {
		if c.Target == alignKey {
			return cell(row, c)
		}
	}
	return ""
}

// coerceCell converts a table cell string to its column type. number and
// bool columns coerce; everything else stays a string (matching how the
// renderer string-coerces string/dropdown/date columns).
func coerceCell(opt any, raw string) any {
	t := "string"
	if m, ok := opt.(map[string]any); ok {
		if s, ok2 := m["type"].(string); ok2 && s != "" {
			t = s
		}
	}
	switch t {
	case "number":
		return Coerce(raw, "number", nil)
	case "bool":
		return Coerce(raw, "boolean", nil)
	default:
		return raw
	}
}

