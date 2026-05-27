package csv

import "strings"

// ExportSourceOption is one selectable source key for the export dialog,
// paired with a display label. Value may be a plain field key or a dotted
// "table.column" subkey; the dialog never constructs these itself.
type ExportSourceOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ExportSchema is everything the export dialog needs to render its columns
// for a given alignment, derived entirely on the backend: the default
// column Plan (with an aligned table expanded into per-column rows), the
// Alignable list/table fields for the align dropdown, and the Sources for
// the computed-column picker. The dialog only renders and edits this.
type ExportSchema struct {
	Plan      ExportPlan           `json:"plan"`
	Alignable []ExportSourceOption `json:"alignable"`
	Sources   []ExportSourceOption `json:"sources"`
	Error     string               `json:"error,omitempty"`
}

// PreviewRowResult is the single data row the dialog shows under each
// column. Cells is blank-filled when the template has no entries.
type PreviewRowResult struct {
	Cells []string `json:"cells"`
	Error string   `json:"error,omitempty"`
}

func fieldDisplayLabel(f FieldSpec) string {
	name := f.Label
	if name == "" {
		name = f.Key
	}
	return name + " (" + f.Type + ")"
}

// tableSubkeys turns a table field's columns into dotted source options.
// The subkey is the column's option value; the backend's findOptionIndex
// resolves it back to a cell position at export time.
func tableSubkeys(f FieldSpec) []ExportSourceOption {
	base := f.Label
	if base == "" {
		base = f.Key
	}
	out := make([]ExportSourceOption, 0, len(f.Options))
	for _, o := range f.Options {
		v, l := optionPair(o)
		if v == "" {
			continue
		}
		if l == "" {
			l = v
		}
		out = append(out, ExportSourceOption{
			Value: f.Key + "." + v,
			Label: base + " → " + l,
		})
	}
	return out
}

func isAlignable(f FieldSpec) bool {
	return f.Type == "list" || f.Type == "table"
}

// AlignableFields lists the mappable list/table fields that alignment can
// target.
func AlignableFields(fields []FieldSpec) []ExportSourceOption {
	out := []ExportSourceOption{}
	for _, f := range MappableFields(fields) {
		if isAlignable(f) {
			out = append(out, ExportSourceOption{Value: f.Key, Label: fieldDisplayLabel(f)})
		}
	}
	return out
}

// SourceOptions is the set of source keys a computed column may pull from:
// every mappable field, plus the aligned table's columns as dotted
// subkeys when alignment targets a table.
func SourceOptions(fields []FieldSpec, alignSource string) []ExportSourceOption {
	mappable := MappableFields(fields)
	out := make([]ExportSourceOption, 0, len(mappable))
	for _, f := range mappable {
		out = append(out, ExportSourceOption{Value: f.Key, Label: fieldDisplayLabel(f)})
	}
	for _, f := range mappable {
		if f.Key == alignSource && f.Type == "table" {
			out = append(out, tableSubkeys(f)...)
		}
	}
	return out
}

// DefaultExportPlan builds the starting column plan for an alignment. With
// no alignment (or list alignment) every mappable field is one column.
// When alignment targets a table, that table expands into one column per
// sub-field so the export is a flat sheet. AlignSource is echoed back only
// when it resolves to a real list/table field.
func DefaultExportPlan(fields []FieldSpec, alignSource string) ExportPlan {
	mappable := MappableFields(fields)
	alignKey, expandTable := "", false
	for _, f := range mappable {
		if f.Key == alignSource && isAlignable(f) {
			alignKey = f.Key
			expandTable = f.Type == "table"
		}
	}
	cols := make([]ExportColumn, 0, len(mappable))
	for _, f := range mappable {
		if expandTable && f.Key == alignKey {
			for _, sub := range tableSubkeys(f) {
				cols = append(cols, ExportColumn{
					Header:     strings.Replace(sub.Value, ".", "-", 1),
					SourceKeys: []string{sub.Value},
				})
			}
			continue
		}
		cols = append(cols, ExportColumn{Header: f.Key, SourceKeys: []string{f.Key}})
	}
	return ExportPlan{Columns: cols, AlignSource: alignKey}
}

// BuildExportSchema bundles the default plan, alignable fields, and source
// options for a single alignment choice.
func BuildExportSchema(fields []FieldSpec, alignSource string) ExportSchema {
	return ExportSchema{
		Plan:      DefaultExportPlan(fields, alignSource),
		Alignable: AlignableFields(fields),
		Sources:   SourceOptions(fields, alignSource),
	}
}
