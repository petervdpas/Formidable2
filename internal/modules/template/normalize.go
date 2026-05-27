package template

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// textareaFormats mirrors `schemas/field.schema.js`:
//   const textareaFormats = new Set(["markdown", "plain"]);
var textareaFormats = map[string]bool{
	"markdown": true,
	"plain":    true,
}

// Normalize coerces a Template's fields into the shape the rest of the
// pipeline (and downstream renderers) expects, mirroring the original
// JS field-schema normalizer. Idempotent: safe to call repeatedly.
//
// Currently handles:
//   - textarea: Format is forced to "markdown" or "plain" (default
//     "markdown" when missing/unknown), case-insensitively.
//   - non-textarea: Format is stripped - the YAML omitempty tag then
//     keeps it out of the saved file.
//
// nil Template / nil Fields are no-ops (callers might pass partials).
func Normalize(t *Template) {
	if t == nil {
		return
	}
	for i := range t.Fields {
		normalizeField(&t.Fields[i])
	}
	normalizeStatistics(t)
	t.Fields = assignLevelScopes(t.Fields)
	for i := range t.Fields {
		if t.Fields[i].LevelScope > 0 && t.Fields[i].ExpressionItem {
			t.Fields[i].ExpressionItem = false
		}
		if t.Fields[i].Type == "guid" {
			t.Fields[i].Key = "id"
		}
		stripDisabledAttributes(&t.Fields[i])
	}
}

// normalizeStatistics is the authoritative cleanup for a template's
// statistical objects (the Statistical Engine's per-template specs):
// trim names, drop entries that are none of a DSL object, a composite
// (needs a parent) or a scaling (needs a source key), and dedupe by name
// (first wins, order preserved). It does NOT parse the DSL or resolve the
// composite / scaling here - that keeps the template package decoupled from
// the Statistical Engine, which reports DSL / relation errors at evaluation.
func normalizeStatistics(t *Template) {
	if len(t.Statistics) == 0 {
		t.Statistics = nil
		return
	}
	seen := map[string]bool{}
	kept := make([]Statistic, 0, len(t.Statistics))
	for _, s := range t.Statistics {
		name := strings.TrimSpace(s.Name)
		dsl := strings.TrimSpace(s.DSL)
		composite := s.Composite != nil && strings.TrimSpace(s.Composite.Parent) != ""
		scaling := s.Scaling != nil && strings.TrimSpace(s.Scaling.Source.Key) != ""
		if name == "" || (dsl == "" && !composite && !scaling) || seen[name] {
			continue
		}
		seen[name] = true
		s.Name = name
		s.DSL = dsl
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		t.Statistics = nil
		return
	}
	t.Statistics = kept
}

func stripDisabledAttributes(f *Field) {
	def, ok := fieldDescriptors[f.Type]
	if !ok {
		return
	}
	for _, attr := range allEnforcedAttrs {
		if !def.Abilities.abilityFor(attr) {
			clearProperty(f, attr)
		}
	}
}

func clearProperty(f *Field, attr string) {
	switch attr {
	case attrSummaryField:
		f.SummaryField = ""
	case attrPrimaryKey:
		f.PrimaryKey = false
	case attrLabel:
		f.Label = ""
	case attrDescription:
		f.Description = ""
	case attrDefault:
		f.Default = nil
	case attrOptions:
		f.Options = nil
	case attrExpressionItem:
		f.ExpressionItem = false
	case attrTwoColumn:
		f.TwoColumn = false
	case attrCollapsible:
		f.Collapsible = nil
	case attrReadonly:
		f.Readonly = false
	case attrFormat:
		f.Format = ""
	case attrUseInStatistics:
		f.UseInStatistics = false
		f.StatisticsColumns = nil
	}
}

func normalizeField(f *Field) {
	coerceDefault(f)
	normalizeStatisticsColumns(f)
	if f.Type == "textarea" {
		canon := strings.ToLower(strings.TrimSpace(f.Format))
		if !textareaFormats[canon] {
			canon = "markdown"
		}
		f.Format = canon
		return
	}
	// All non-textarea types: format has no meaning, drop it.
	f.Format = ""
}

// normalizeStatisticsColumns is the authoritative cleanup for a table
// field's per-column statistics selection. It clears the list for any
// non-table field or a field that isn't flagged use_in_statistics, then
// dedupes the remainder and drops entries that don't name a real column
// (matched against each option's `value`), preserving first-seen order.
// The FieldEditModal applies the same constraints as a UX accelerator;
// this pass is the source of truth, so manual YAML / imports / plugins
// can't persist duplicate or dangling column keys.
func normalizeStatisticsColumns(f *Field) {
	if !f.UseInStatistics || f.Type != "table" {
		f.StatisticsColumns = nil
		return
	}
	if len(f.StatisticsColumns) == 0 {
		f.StatisticsColumns = nil
		return
	}
	valid := map[string]bool{}
	for _, opt := range f.Options {
		switch o := opt.(type) {
		case map[string]any:
			if v, _ := o["value"].(string); v != "" {
				valid[v] = true
			}
		case string:
			if o != "" {
				valid[o] = true
			}
		}
	}
	seen := map[string]bool{}
	kept := make([]string, 0, len(f.StatisticsColumns))
	for _, c := range f.StatisticsColumns {
		if !valid[c] || seen[c] {
			continue
		}
		seen[c] = true
		kept = append(kept, c)
	}
	if len(kept) == 0 {
		f.StatisticsColumns = nil
		return
	}
	f.StatisticsColumns = kept
}

// coerceDefault forces f.Default into the Go type the field's data
// values will take. Without this pass, a YAML/CSV/plugin-authored
// template that stored Default as the wrong type would seed every new
// form with mis-typed data - the exact path that produced the
// "expected number, got string" drift on the `numpy` field. The
// frontend FieldEditModal does the same coercion as a UX accelerator;
// this backend pass is the authoritative source of truth.
//
// nil defaults pass through unchanged. Unparseable inputs (e.g. the
// string "seventeen" in a number field) clear to nil so storage.Sanitize
// falls back to defaultForType on first save.
func coerceDefault(f *Field) {
	if f.Default == nil {
		return
	}
	switch f.Type {
	case "number", "range":
		f.Default = coerceNumber(f.Default)
	case "boolean":
		f.Default = coerceBool(f.Default)
	case "tags", "multioption", "list":
		f.Default = coerceArray(f.Default)
	case "date":
		f.Default = coerceDate(f.Default)
	case "text", "textarea", "dropdown", "radio",
		"file-path", "folder-path", "image":
		// link is intentionally absent - its Default may legitimately
		// be {href, text} map OR a legacy string, so the "non-string
		// → string" rule doesn't apply.
		f.Default = coerceTextShape(f.Default)
	}
}

func coerceNumber(v any) any {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return f
	}
	return nil
}

func coerceBool(v any) any {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "yes", "1", "on":
			return true
		case "false", "no", "0", "off":
			return false
		case "":
			return nil
		}
	}
	return nil
}

func coerceArray(v any) any {
	switch x := v.(type) {
	case []any:
		return x
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return []any{}
		}
		parts := strings.FieldsFunc(s, func(r rune) bool {
			return r == ',' || r == ';'
		})
		out := make([]any, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			out = append(out, p)
		}
		return out
	}
	return v
}

// dateLayouts is the parse-attempt order for date-typed defaults. ISO
// is first since it's the wire shape; locale-style fallbacks come
// after. Output is always ISO YYYY-MM-DD regardless of input.
var dateLayouts = []string{
	"2006-01-02",
	"02/01/2006",
	"01/02/2006",
	"02-01-2006",
	"01-02-2006",
	"2006/01/02",
}

func coerceDate(v any) any {
	switch x := v.(type) {
	case time.Time:
		return x.Format("2006-01-02")
	case string:
		s := strings.TrimSpace(cleanText(x))
		if s == "" {
			return nil
		}
		for _, layout := range dateLayouts {
			if t, err := time.Parse(layout, s); err == nil {
				return t.Format("2006-01-02")
			}
		}
		return nil
	}
	return nil
}

func coerceTextShape(v any) any {
	switch x := v.(type) {
	case string:
		s := cleanText(x)
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return s
	case nil:
		return nil
	}
	// Non-string non-nil (e.g. YAML wrote `default: 42` on a text
	// field). Render it as its string form so the wire shape matches
	// what the field component expects on load.
	return fmt.Sprint(v)
}

// cleanText strips invisible characters that paste from Word / web /
// PDF rich-text controls injects but a human typing in a plain text
// field never intends:
//
//   - U+00A0 non-breaking space → regular space
//   - U+200B zero-width space    → removed
//   - U+200C zero-width non-joiner → removed
//   - U+200D zero-width joiner   → removed
//   - U+2060 word joiner         → removed
//   - U+FEFF BOM / zero-width nbsp → removed
//
// Smart quotes, em-dashes, en-dashes, ellipsis etc. are NOT touched -
// those are often deliberate in human-written content.
func cleanText(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\u00A0': // non-breaking space → regular space
			b.WriteByte(' ')
		case '\u200B', // zero-width space
			'\u200C', // zero-width non-joiner
			'\u200D', // zero-width joiner
			'\u2060', // word joiner
			'\uFEFF': // BOM / zero-width nbsp
			// drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
