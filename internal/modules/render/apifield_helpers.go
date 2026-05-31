package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// registerAPIFieldHelpers binds {{apiCol}}, {{apiGuid}}, {{apiBlock}},
// {{apiSection}}. All read the host api Field from the context's `_fields`
// (so they work inside `{{#loop}}` too) and degrade gracefully on a
// missing/non-api field, an unpicked record, or a nil LoadTemplate.
func registerAPIFieldHelpers(tpl *raymond.Template, opts *Options) {
	// {{apiCol "fieldKey" "columnKey"}}: one projected column, scalar
	// inline or compact JSON for slice/map.
	tpl.RegisterHelper("apiCol", func(fieldKey, columnKey string, options *raymond.Options) raymond.SafeString {
		row, _ := apiFieldRow(options.Ctx(), fieldKey)
		if row == nil {
			return ""
		}
		v, ok := row[columnKey]
		if !ok || v == nil {
			return ""
		}
		return raymond.SafeString(scalarOrJSON(v))
	})

	// {{apiGuid "fieldKey"}}: the picked record's guid, or "".
	tpl.RegisterHelper("apiGuid", func(fieldKey string, options *raymond.Options) string {
		row, _ := apiFieldRow(options.Ctx(), fieldKey)
		if row == nil {
			return ""
		}
		if g, ok := row["guid"].(string); ok {
			return g
		}
		return ""
	})

	// {{apiBlock "fieldKey" "columnKey"}}: type-aware render (tags joined,
	// list bulleted, table as a pipe-table), per the source field's type;
	// falls back to scalarOrJSON without a loadable source.
	tpl.RegisterHelper("apiBlock", func(fieldKey, columnKey string, options *raymond.Options) raymond.SafeString {
		row, hostField := apiFieldRow(options.Ctx(), fieldKey)
		if row == nil {
			return ""
		}
		v, ok := row[columnKey]
		if !ok || v == nil {
			return ""
		}
		src := loadSourceField(opts, hostField, columnKey)
		return raymond.SafeString(emitAPIColumnBlock(v, src))
	})

	// {{apiSection "fieldKey"}}: full embedded-card markdown (header plus
	// a line per column). The drop-everything helper for generator output.
	tpl.RegisterHelper("apiSection", func(fieldKey string, options *raymond.Options) raymond.SafeString {
		row, hostField := apiFieldRow(options.Ctx(), fieldKey)
		if row == nil || hostField == nil {
			return ""
		}
		return raymond.SafeString(emitAPISection(row, hostField, opts))
	})
}

// apiFieldRow returns the host api field's value (a {guid, ...columns} map)
// and the Field itself (for its Map[]); (nil, nil) when missing or unpicked.
// Loop-aware via contextMap/findField, so it sees the per-iteration context.
func apiFieldRow(ctx any, fieldKey string) (map[string]any, *template.Field) {
	cm := contextMap(ctx)
	if cm == nil {
		return nil, nil
	}
	f := findField(ctx, fieldKey)
	if f == nil || f.Type != "api" {
		return nil, f
	}
	v, ok := cm[fieldKey]
	if !ok || v == nil {
		return nil, f
	}
	row, ok := v.(map[string]any)
	if !ok {
		return nil, f
	}
	return row, f
}

// scalarOrJSON returns a string/number/bool as-is, else compact JSON for
// safe inline insertion. Mirrors FormFieldAPI.vue's display() rule.
func scalarOrJSON(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprintf("%v", x)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// loadSourceField resolves the source-template Field backing one Map[]
// entry; nil when the loader/source/column is missing (e.g. a since-removed field).
func loadSourceField(opts *Options, hostField *template.Field, columnKey string) *template.Field {
	if opts == nil || opts.LoadTemplate == nil {
		return nil
	}
	if hostField == nil || hostField.Collection == "" {
		return nil
	}
	src := opts.LoadTemplate(hostField.Collection)
	if src == nil {
		return nil
	}
	for i := range src.Fields {
		if src.Fields[i].Key == columnKey {
			return &src.Fields[i]
		}
	}
	return nil
}

// emitAPIColumnBlock dispatches on the source field type: tags joined,
// list bulleted, table as a pipe-table, else scalarOrJSON. A nil source
// falls back to scalarOrJSON so cross-template renames don't crash.
func emitAPIColumnBlock(v any, source *template.Field) string {
	if source == nil {
		return scalarOrJSON(v)
	}
	switch source.Type {
	case "tags":
		if arr, ok := v.([]any); ok {
			parts := make([]string, 0, len(arr))
			for _, item := range arr {
				parts = append(parts, scalarOrJSON(item))
			}
			return strings.Join(parts, ", ")
		}
	case "list":
		if arr, ok := v.([]any); ok {
			if len(arr) == 0 {
				return ""
			}
			lines := make([]string, 0, len(arr))
			for _, item := range arr {
				lines = append(lines, "- "+scalarOrJSON(item))
			}
			return strings.Join(lines, "\n")
		}
	case "table", "multioption":
		if arr, ok := v.([]any); ok {
			return emitMarkdownTable(arr, source)
		}
	}
	return scalarOrJSON(v)
}

// emitMarkdownTable renders a stored 2-D table as a markdown pipe-table,
// headers from source.Options[].label. Handles both stored shapes:
// array-of-arrays (positional cells) and array-of-objects (keyed by
// Options[].value). Empty headers or rows yield "".
func emitMarkdownTable(rows []any, source *template.Field) string {
	if len(rows) == 0 {
		return ""
	}
	// Column metadata: parallel arrays of header labels + lookup keys.
	type col struct{ label, key string }
	cols := []col{}
	for _, opt := range source.Options {
		m, ok := opt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m["value"].(string)
		label, _ := m["label"].(string)
		if label == "" {
			label = key
		}
		cols = append(cols, col{label: label, key: key})
	}
	if len(cols) == 0 {
		return scalarOrJSON(rows) // no column metadata: bail to JSON
	}

	var b strings.Builder
	b.WriteString("|")
	for _, c := range cols {
		b.WriteString(" ")
		b.WriteString(escapePipe(c.label))
		b.WriteString(" |")
	}
	b.WriteString("\n|")
	for range cols {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")

	for _, raw := range rows {
		b.WriteString("|")
		switch r := raw.(type) {
		case []any: // positional cells
			for i := range cols {
				var cell any
				if i < len(r) {
					cell = r[i]
				}
				b.WriteString(" ")
				b.WriteString(escapePipe(scalarOrJSON(cell)))
				b.WriteString(" |")
			}
		case map[string]any: // keyed cells (alternate shape)
			for _, c := range cols {
				cell := r[c.key]
				b.WriteString(" ")
				b.WriteString(escapePipe(scalarOrJSON(cell)))
				b.WriteString(" |")
			}
		default: // unrecognised shape: JSON across all cells for debugging
			for range cols {
				b.WriteString(" ")
				b.WriteString(escapePipe(scalarOrJSON(raw)))
				b.WriteString(" |")
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// emitAPISection renders the embedded card wrapped in `<section
// class="api-card">`. The blank lines around the tags hit goldmark's
// type-6 HTML-block rule so the inner markdown still parses (same trick
// the loop-item wrappers use). Block-shaped values (table/list) go on
// their own lines after the column header to keep their markdown semantics.
func emitAPISection(row map[string]any, hostField *template.Field, opts *Options) string {
	if hostField == nil {
		return ""
	}
	hostLabel := hostField.Label
	if hostLabel == "" {
		hostLabel = hostField.Key
	}

	var b strings.Builder
	// data-source mirrors the in-app .api-field-card naming, for theming.
	b.WriteString(`<section class="api-card" data-source="`)
	b.WriteString(hostField.Collection)
	b.WriteString("\">\n\n")

	b.WriteString("**")
	b.WriteString(hostLabel)
	b.WriteString("** _(")
	b.WriteString(hostField.Collection)
	b.WriteString(")_")
	b.WriteString("\n\n")

	for _, m := range hostField.Map {
		colLabel := strings.TrimSpace(m.Label)
		if colLabel == "" {
			colLabel = m.Key
		}
		v := row[m.Key]
		src := loadSourceField(opts, hostField, m.Key)
		// Block types get the header on its own line, then the block.
		if isAPIBlockType(src) {
			b.WriteString("- **")
			b.WriteString(colLabel)
			b.WriteString("**:\n\n")
			b.WriteString(emitAPIColumnBlock(v, src))
			b.WriteString("\n\n")
			continue
		}
		b.WriteString("- **")
		b.WriteString(colLabel)
		b.WriteString("**: ")
		b.WriteString(emitAPIColumnBlock(v, src))
		b.WriteString("\n")
	}

	// Blank-line gap so goldmark resumes block parsing after the section.
	b.WriteString("\n</section>")
	return b.String()
}

func isAPIBlockType(src *template.Field) bool {
	if src == nil {
		return false
	}
	switch src.Type {
	case "table", "list", "multioption":
		return true
	}
	return false
}

func escapePipe(s string) string {
	// Pipes and newlines both break a table cell: escape the pipe, flatten newlines.
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
