package template

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var textareaFormats = map[string]bool{
	"markdown": true,
	"plain":    true,
}

var facetFormats = map[string]bool{
	"radio":    true,
	"dropdown": true,
}

// Normalize coerces a Template's fields into the canonical shape the pipeline expects. Idempotent.
func Normalize(t *Template) {
	if t == nil {
		return
	}
	for i := range t.Fields {
		normalizeField(&t.Fields[i])
	}
	migrateLegacyScalings(t)
	normalizeStatistics(t)
	normalizeScalings(t)
	normalizeFormulas(t)
	normalizeFacetFieldDefaults(t)
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

// normalizeFacetFieldDefaults clears each facet field's Default unless it names a known option label.
// Runs at template scope because the per-field pass has no access to t.Facets.
func normalizeFacetFieldDefaults(t *Template) {
	if t == nil {
		return
	}
	options := map[string]map[string]bool{}
	for _, f := range t.Facets {
		opts := make(map[string]bool, len(f.Options))
		for _, o := range f.Options {
			opts[o.Label] = true
		}
		options[f.Key] = opts
	}
	for i := range t.Fields {
		f := &t.Fields[i]
		if f.Type != "facet" {
			continue
		}
		s, ok := f.Default.(string)
		if !ok || s == "" {
			f.Default = nil
			continue
		}
		known, ok := options[f.FacetKey]
		if !ok || !known[s] {
			f.Default = nil
		}
	}
}

// normalizeStatistics trims names, drops empty entries, and dedupes by name (first wins). It does NOT
// parse the DSL or resolve composites, keeping this package decoupled from the Statistical Engine.
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
	case attrFacetKey:
		f.FacetKey = ""
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
	if f.Type == "facet" {
		canon := strings.ToLower(strings.TrimSpace(f.Format))
		if !facetFormats[canon] {
			canon = "radio"
		}
		f.Format = canon
		return
	}
	// Format has no meaning on other types.
	f.Format = ""
}

// normalizeStatisticsColumns dedupes a table field's stat-column selection and drops keys that don't name
// a real column; it's the source of truth so manual YAML / imports / plugins can't persist dangling keys.
// normalizeFormulas trims each formula, drops entries with no key or no
// expression, dedups by key (first wins), and defaults a blank type to
// "number" (the common case and the only one that aggregates numerically).
func normalizeFormulas(t *Template) {
	if len(t.Formulas) == 0 {
		t.Formulas = nil
		return
	}
	seen := map[string]bool{}
	kept := make([]Formula, 0, len(t.Formulas))
	for _, f := range t.Formulas {
		key := strings.TrimSpace(f.Key)
		expr := strings.TrimSpace(f.Expression)
		if key == "" || expr == "" || seen[key] {
			continue
		}
		seen[key] = true
		f.Key = key
		f.Expression = expr
		f.Label = strings.TrimSpace(f.Label)
		f.Type = strings.TrimSpace(f.Type)
		if f.Type == "" {
			f.Type = "number"
		}
		kept = append(kept, f)
	}
	if len(kept) == 0 {
		t.Formulas = nil
		return
	}
	t.Formulas = kept
}

// migrateLegacyScalings lifts any scaling stored under the legacy
// statistics[].scaling shape into the top-level t.Scalings catalog (carrying
// the enclosing Name/Label) and removes it from t.Statistics. In-memory only:
// it does not rewrite the user's YAML; the new scalings: shape persists on the
// next in-app save (same posture as the facets legacy migration). Returns true
// when it moved at least one entry, so the load path can flag a resave.
func migrateLegacyScalings(t *Template) bool {
	if len(t.Statistics) == 0 {
		return false
	}
	moved := false
	kept := make([]Statistic, 0, len(t.Statistics))
	for _, s := range t.Statistics {
		if s.Scaling != nil && strings.TrimSpace(s.Scaling.Source.Key) != "" {
			t.Scalings = append(t.Scalings, Scaling{
				Name:    s.Name,
				Label:   s.Label,
				Source:  s.Scaling.Source,
				Weights: s.Scaling.Weights,
				Default: s.Scaling.Default,
			})
			moved = true
			continue
		}
		kept = append(kept, s)
	}
	t.Statistics = kept
	return moved
}

// normalizeScalings trims names, drops entries with no name or no source key,
// and dedupes by name (first wins). It does not validate the source kind or the
// weights; Validate does that.
func normalizeScalings(t *Template) {
	if len(t.Scalings) == 0 {
		t.Scalings = nil
		return
	}
	seen := map[string]bool{}
	kept := make([]Scaling, 0, len(t.Scalings))
	for _, s := range t.Scalings {
		name := strings.TrimSpace(s.Name)
		if name == "" || strings.TrimSpace(s.Source.Key) == "" || seen[name] {
			continue
		}
		seen[name] = true
		s.Name = name
		s.Label = strings.TrimSpace(s.Label)
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		t.Scalings = nil
		return
	}
	t.Scalings = kept
}

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

// coerceDefault forces f.Default into the field's Go type; without it a wrong-typed Default would seed
// every new form mis-typed (the "expected number, got string" drift). Unparseable inputs clear to nil
// so storage.Sanitize falls back to defaultForType on first save.
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
		// link is intentionally absent: its Default may be a {href, text} map, not just a string.
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

// dateLayouts is the parse-attempt order for date-typed defaults (ISO first); output is always ISO.
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
	// Non-string non-nil (e.g. YAML `default: 42` on a text field): render to string for the wire shape.
	return fmt.Sprint(v)
}

// cleanText converts U+00A0 to a regular space and strips zero-width characters that rich-text paste
// injects but a human never intends. Smart quotes / dashes / ellipsis are left alone (often deliberate).
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
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
