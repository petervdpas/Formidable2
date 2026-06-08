package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// registerAPIFieldHelpers binds {{apiCol}}, {{apiGuid}}, {{apiBlock}},
// {{apiSection}}. The field stores only the reference id(s); the columns are
// read live via Options.ResolveReference. All read the host api Field from the
// context's `_fields` (so they work inside `{{#loop}}` too) and degrade
// gracefully on a missing/non-api field, an unpicked record, or a nil resolver.
// The single-value helpers act on the first referenced record; apiSection
// renders one card per referenced record (so a to-many reference shows them all).
// The polymorphic {{field}} helper also drives api fields: `{{field "k"}}` for
// cards, `{{field "k" mode=table}}` for the full pipe-table, and the dotted path
// `{{field "k.column"}}` for one projected column (see helpers_field.go).
func registerAPIFieldHelpers(tpl *raymond.Template, opts *Options) {
	// {{apiCol "fieldKey" "columnKey"}}: one projected column, scalar
	// inline or compact JSON for slice/map.
	tpl.RegisterHelper("apiCol", func(fieldKey, columnKey string, options *raymond.Options) raymond.SafeString {
		ids, host := apiFieldRefs(options.Ctx(), fieldKey)
		if len(ids) == 0 {
			return ""
		}
		row := apiResolve(opts, host, ids[0], []string{columnKey})
		v, ok := row[columnKey]
		if !ok || v == nil {
			return ""
		}
		return raymond.SafeString(scalarOrJSON(v))
	})

	// {{apiGuid "fieldKey"}}: the referenced id(s); a single id, or ids joined
	// by ", " for a to-many reference. "" when unpicked.
	tpl.RegisterHelper("apiGuid", func(fieldKey string, options *raymond.Options) string {
		ids, _ := apiFieldRefs(options.Ctx(), fieldKey)
		return strings.Join(ids, ", ")
	})

	// {{apiRows "fieldKey"}}: the referenced records for `{{#each (apiRows "k")}}`.
	// Each row is a map keyed by Map column key for named access (`{{this.term}}`
	// or bare `{{term}}`), plus `link` (the record's formidable:// deep link,
	// `{{this.link}}`) and `labels` (column key -> Map label, `{{this.labels.term}}`).
	// The header row is `{{fieldMeta "fieldKey" "options"}}`.
	tpl.RegisterHelper("apiRows", func(fieldKey string, options *raymond.Options) []any {
		ids, host := apiFieldRefs(options.Ctx(), fieldKey)
		if host == nil || len(ids) == 0 {
			return nil
		}
		cols := columnKeysOf(host)
		labels := apiColumnLabels(host)
		rows := make([]any, 0, len(ids))
		for _, id := range ids {
			row := apiResolve(opts, host, id, cols)
			if row == nil {
				continue
			}
			m := make(map[string]any, len(row)+2)
			for k, v := range row {
				m[k] = v
			}
			m["link"] = apiRecordLink(opts, host, id)
			m["labels"] = labels
			rows = append(rows, m)
		}
		return rows
	})

	// {{apiBlock "fieldKey" "columnKey"}}: type-aware render (tags joined,
	// list bulleted, table as a pipe-table), per the source field's type;
	// falls back to scalarOrJSON without a loadable source.
	tpl.RegisterHelper("apiBlock", func(fieldKey, columnKey string, options *raymond.Options) raymond.SafeString {
		ids, host := apiFieldRefs(options.Ctx(), fieldKey)
		if len(ids) == 0 {
			return ""
		}
		row := apiResolve(opts, host, ids[0], []string{columnKey})
		v, ok := row[columnKey]
		if !ok || v == nil {
			return ""
		}
		src := loadSourceField(opts, host, columnKey)
		return raymond.SafeString(emitAPIColumnBlock(v, src))
	})

	// {{apiSection "fieldKey"}}: full embedded-card markdown (header plus a line
	// per column), one card per referenced record. The drop-everything helper.
	tpl.RegisterHelper("apiSection", func(fieldKey string, options *raymond.Options) raymond.SafeString {
		ids, host := apiFieldRefs(options.Ctx(), fieldKey)
		if host == nil || len(ids) == 0 {
			return ""
		}
		cols := columnKeysOf(host)
		parts := make([]string, 0, len(ids))
		for _, id := range ids {
			row := apiResolve(opts, host, id, cols)
			if row == nil {
				continue
			}
			parts = append(parts, emitAPISection(id, row, host, opts))
		}
		return raymond.SafeString(strings.Join(parts, "\n\n"))
	})
}

// apiFieldRefs returns the host api field's referenced id(s) and the Field
// itself (for its Map[]). Loop-aware via contextMap/findField, so it sees the
// per-iteration context. (nil, field) when unpicked; (nil, nil) when missing.
func apiFieldRefs(ctx any, fieldKey string) ([]string, *template.Field) {
	cm := contextMap(ctx)
	if cm == nil {
		return nil, nil
	}
	f := findField(ctx, fieldKey)
	if f == nil || f.Type != "api" {
		return nil, f
	}
	return refIDsFromValue(cm[fieldKey]), f
}

// refIDsFromValue pulls the reference id(s) from an api-field value: a bare id
// string (single), a list of id strings (to-many), or the legacy {id|guid, ...}
// snapshot (tolerated for not-yet-healed data). Empty entries are dropped.
func refIDsFromValue(v any) []string {
	switch t := v.(type) {
	case string:
		if t != "" {
			return []string{t}
		}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			switch ev := e.(type) {
			case string:
				if ev != "" {
					out = append(out, ev)
				}
			case map[string]any:
				if id := legacyRefID(ev); id != "" {
					out = append(out, id)
				}
			}
		}
		return out
	case map[string]any:
		if id := legacyRefID(t); id != "" {
			return []string{id}
		}
	}
	return nil
}

// legacyRefID pulls the id out of a legacy api snapshot map (id, then guid).
func legacyRefID(m map[string]any) string {
	if s, ok := m["id"].(string); ok && s != "" {
		return s
	}
	if s, ok := m["guid"].(string); ok && s != "" {
		return s
	}
	return ""
}

// columnKeysOf returns the host field's Map column keys (the subset to project).
func columnKeysOf(host *template.Field) []string {
	if host == nil {
		return nil
	}
	keys := make([]string, 0, len(host.Map))
	for _, m := range host.Map {
		if m.Key != "" {
			keys = append(keys, m.Key)
		}
	}
	return keys
}

// apiRecordLink resolves the referenced record's formidable:// deep link and
// rewrites it per target; "" when no resolver is wired, the host has no
// collection, or the id is empty (so the card header stays plain text).
func apiRecordLink(opts *Options, host *template.Field, id string) string {
	if opts == nil || opts.ResolveReferenceLink == nil || host == nil || host.Collection == "" || id == "" {
		return ""
	}
	return resolveLinkHref(opts.ResolveReferenceLink(host.Collection, id), opts)
}

// apiResolve projects one target record live into a row keyed by cols; nil when
// the resolver, host, or id is missing, or the record is gone (volatile).
func apiResolve(opts *Options, host *template.Field, id string, cols []string) map[string]any {
	if opts == nil || opts.ResolveReference == nil || host == nil || host.Collection == "" || id == "" {
		return nil
	}
	return opts.ResolveReference(host.Collection, id, cols)
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
// The first scalar column's value links to the referenced record (the rendered
// "Go to record") when a link resolver is wired.
func emitAPISection(id string, row map[string]any, hostField *template.Field, opts *Options) string {
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

	// resolveLinkHref rewrites formidable:// per target (slideout keeps it,
	// wiki/pdf rewrite); placed on the first scalar column's value below.
	link := apiRecordLink(opts, hostField, id)
	linked := false

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
		cell := emitAPIColumnBlock(v, src)
		if !linked && link != "" && cell != "" {
			cell = "[" + cell + "](" + link + ")"
			linked = true
		}
		b.WriteString("- **")
		b.WriteString(colLabel)
		b.WriteString("**: ")
		b.WriteString(cell)
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

// apiColumnLabels maps each Map column key to its display label (the Map alias,
// else the key). Shared by every {{apiRows}} row so `{{this.labels.term}}` works.
func apiColumnLabels(host *template.Field) map[string]any {
	if host == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(host.Map))
	for _, m := range host.Map {
		if m.Key == "" {
			continue
		}
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = m.Key
		}
		out[m.Key] = label
	}
	return out
}

// apiColumnOptions exposes the api field's Map columns in a table field's
// {value,label} Options shape, so the header idiom `{{#each (fieldMeta "k"
// "options")}}{{label}}{{/each}}` renders identically for api fields.
func apiColumnOptions(host *template.Field) []any {
	if host == nil {
		return nil
	}
	out := make([]any, 0, len(host.Map))
	for _, m := range host.Map {
		if m.Key == "" {
			continue
		}
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = m.Key
		}
		out = append(out, map[string]any{"value": m.Key, "label": label})
	}
	return out
}
