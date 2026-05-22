package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// registerAPIFieldHelpers binds the four api-field helpers
// ({{apiCol}}, {{apiGuid}}, {{apiBlock}}, {{apiSection}}). Split out
// of helpers.go so the api dispatch stays grouped with its support
// functions.
//
// All four read the host's api Field from the current context's
// `_fields` (so they work both at top-level and inside `{{#loop}}`
// - the same context-walking the `{{field}}` helper relies on).
// Each helper degrades gracefully when:
//   - the field doesn't exist or isn't api-typed
//   - the modelValue is missing / nil (no record picked yet)
//   - Options.LoadTemplate is nil (apiBlock/apiSection fall back to
//     scalar/JSON behavior - apiCol/apiGuid don't need it)
func registerAPIFieldHelpers(tpl *raymond.Template, opts *Options) {
	// {{apiCol "fieldKey" "columnKey"}} - read one projected column.
	// Scalar value passes through (string/number/bool); non-scalar
	// (slice/map) renders as a compact JSON string. Inline-friendly.
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

	// {{apiGuid "fieldKey"}} - return the picked record's guid string.
	// Empty when no record has been picked.
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

	// {{apiBlock "fieldKey" "columnKey"}} - type-aware block render.
	// Loads the source template via Options.LoadTemplate to read the
	// column's source field type:
	//   - scalar → same as apiCol
	//   - tags   → comma-joined string
	//   - list   → markdown bullets
	//   - table  → markdown table with headers from the source field's
	//     options[].label
	//
	// Falls back to scalarOrJSON when LoadTemplate is nil or the
	// source can't be loaded.
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

	// {{apiSection "fieldKey"}} - full embedded card markdown:
	// "**<host label>** _(source)_" header + per-column "**<label>**:
	// <value-or-block>" lines. The lazy "drop everything in here"
	// helper for Report/Minimal generator output.
	tpl.RegisterHelper("apiSection", func(fieldKey string, options *raymond.Options) raymond.SafeString {
		row, hostField := apiFieldRow(options.Ctx(), fieldKey)
		if row == nil || hostField == nil {
			return ""
		}
		return raymond.SafeString(emitAPISection(row, hostField, opts))
	})
}

// apiFieldRow looks up the host's api Field by key in the current
// context's `_fields` slot, returns its current value (a
// {guid, ...projected_columns} map) plus a pointer to the host Field
// itself so callers can read its Map[]. Returns (nil, nil) when the
// field is missing or doesn't carry a record yet.
//
// Loop-aware via contextMap/findField - when invoked inside `{{#loop}}`
// the helper sees the per-iteration context; the inner api field's
// row comes from that iteration's value.
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

// scalarOrJSON returns the value as-is when it's a Handlebars-friendly
// scalar (string/number/bool); otherwise renders a compact JSON string
// for safe inline insertion. Mirrors the frontend's `display()` rule
// in FormFieldAPI.vue so MD output and the in-app card stay aligned.
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

// loadSourceField resolves the source-template Field that backs one
// of the host api field's Map[] entries. Returns nil when the loader
// is missing, the source can't be loaded, or the column key has no
// matching field in the source roster (e.g. user removed a field
// after the api field's Map[] was configured).
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

// emitAPIColumnBlock is the type-aware renderer for {{apiBlock}}. It
// inspects the source field's type and dispatches:
//
//   - tags  → comma-joined string ("a, b, c")
//   - list  → markdown bullet list (one item per line)
//   - table → markdown pipe-table with header row from
//     source.Options[].label and body rows from the value
//   - everything else → scalarOrJSON fallback (safe in any context)
//
// nil source field falls back to scalarOrJSON so cross-template
// renames don't crash the renderer.
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

// emitMarkdownTable renders a stored 2-D table value as a markdown
// pipe-table. Headers come from `source.Options[].label` (the
// authoritative column list also used by the in-app FormFieldTable
// renderer) so a host's MD output and on-screen card stay in sync.
//
// Supports both stored shapes:
//   - array-of-arrays (FormFieldTable's native shape): one row per
//     entry, cells in declared column order
//   - array-of-objects: cells indexed by source.Options[].value
//
// Empty headers / empty rows produce an empty string (caller decides
// whether to wrap "No data" prose around the call).
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
		// Source has no column metadata - bail to JSON so the user
		// at least sees the data.
		return scalarOrJSON(rows)
	}

	var b strings.Builder
	// Header
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

	// Body rows
	for _, raw := range rows {
		b.WriteString("|")
		switch r := raw.(type) {
		case []any:
			// Positional cells.
			for i := range cols {
				var cell any
				if i < len(r) {
					cell = r[i]
				}
				b.WriteString(" ")
				b.WriteString(escapePipe(scalarOrJSON(cell)))
				b.WriteString(" |")
			}
		case map[string]any:
			// Keyed cells (older / alternate shape).
			for _, c := range cols {
				cell := r[c.key]
				b.WriteString(" ")
				b.WriteString(escapePipe(scalarOrJSON(cell)))
				b.WriteString(" |")
			}
		default:
			// Unrecognised row shape - emit the JSON form across all
			// cells so the user can debug.
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

// emitAPISection renders the full embedded-card markdown wrapped in
// a `<section class="api-card">` container so the goldmark output
// carries card chrome that formidable-prose.css can style. The blank
// lines around the `<section>` tags hit goldmark's "type 6" HTML-
// block rule, which leaves the inner markdown to be parsed normally
// (same trick the loop-item wrappers rely on).
//
// The card body is:
//
//	**<host label>** _(<source filename>)_
//
//	- **<col label>**: <inline-or-block value>
//	- **<col label>**: ...
//
// Block-shaped column values (table/list) are placed on their own
// lines after the column header instead of inline, preserving
// markdown table/list semantics.
func emitAPISection(row map[string]any, hostField *template.Field, opts *Options) string {
	if hostField == nil {
		return ""
	}
	hostLabel := hostField.Label
	if hostLabel == "" {
		hostLabel = hostField.Key
	}

	var b strings.Builder
	// Open card. The data-source attribute is exposed for theming /
	// devtools and matches the in-app .api-field-card naming.
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
		// Tables and lists need their own block - emit the header on
		// its own line, then a blank line, then the block.
		if isAPIBlockType(src) {
			b.WriteString("- **")
			b.WriteString(colLabel)
			b.WriteString("**:\n\n")
			b.WriteString(emitAPIColumnBlock(v, src))
			b.WriteString("\n\n")
			continue
		}
		// Inline value.
		b.WriteString("- **")
		b.WriteString(colLabel)
		b.WriteString("**: ")
		b.WriteString(emitAPIColumnBlock(v, src))
		b.WriteString("\n")
	}

	// Close card with a blank-line gap so goldmark resumes block
	// parsing cleanly after the section.
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
	// Newlines + pipes both break a markdown table cell. Replace
	// pipes with the HTML entity (markdown engines pass it through)
	// and collapse newlines to a space.
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
