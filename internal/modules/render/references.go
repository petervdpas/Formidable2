package render

import (
	"strings"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// templateFields is the whole template's field list from any context (root or
// loop iteration both carry _template); falls back to the context's fields.
func templateFields(ctx any) []template.Field {
	if m := contextMap(ctx); m != nil {
		if t, ok := m["_template"].(*template.Template); ok && t != nil {
			return t.Fields
		}
	}
	return contextFields(ctx)
}

// withReferenceAnchor prefixes a field's output with its in-page anchor when
// some reference column targets that field and the value is non-empty.
func withReferenceAnchor(options *raymond.Options, out any) any {
	params := options.Params()
	if len(params) == 0 {
		return out
	}
	key, _ := params[0].(string)
	ctx := contextMap(options.Ctx())
	if key == "" || ctx == nil {
		return out
	}
	if _, ok := template.ReferenceTargets(templateFields(options.Ctx()))[key]; !ok {
		return out
	}
	id := template.ReferenceAnchor(key, stringify(ctx[key]))
	if id == "" {
		return out
	}
	anchor := `<a id="` + id + `"></a>`
	switch v := out.(type) {
	case raymond.SafeString:
		return raymond.SafeString(anchor + string(v))
	case string:
		return raymond.SafeString(anchor + raymond.Escape(v))
	}
	return raymond.SafeString(anchor + raymond.Escape(stringify(out)))
}

// knownReferenceCodes collects, per target field, the lower-cased values present
// in its loop's items of the root record.
func knownReferenceCodes(ctx map[string]any, targets map[string]string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for target, loop := range targets {
		set := map[string]bool{}
		items, _ := ctx[loop].([]any)
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				if v := strings.TrimSpace(stringify(m[target])); v != "" {
					set[strings.ToLower(v)] = true
				}
			}
		}
		out[target] = set
	}
	return out
}

func tableCellText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

// registerTableHelper binds {{fieldTable "key"}}: a table field as a GFM pipe table
// with column labels as headers. Reference cells link each code that matches an
// existing loop item to its in-page anchor; unmatched codes stay plain text.
func registerTableHelper(tpl *raymond.Template) {
	tpl.RegisterHelper("fieldTable", func(key string, options *raymond.Options) any {
		ctx := contextMap(options.Ctx())
		f := findField(options.Ctx(), key)
		if ctx == nil || f == nil || f.Type != "table" {
			return ""
		}
		rows, _ := ctx[key].([]any)
		if len(rows) == 0 {
			return ""
		}
		refCols := template.ReferenceColumns(*f)
		known := knownReferenceCodes(ctx, template.ReferenceTargets(templateFields(options.Ctx())))
		var b strings.Builder
		b.WriteString("|")
		for _, o := range f.Options {
			val, lab := optionPair(o)
			if lab == "" {
				lab = val
			}
			b.WriteString(" " + tableCellText(lab) + " |")
		}
		b.WriteString("\n|" + strings.Repeat("---|", len(f.Options)) + "\n")
		for _, r := range rows {
			cells, ok := r.([]any)
			if !ok {
				continue
			}
			b.WriteString("|")
			for i := range f.Options {
				text := ""
				if i < len(cells) && cells[i] != nil {
					text = stringify(cells[i])
				}
				if target, isRef := refCols[i]; isRef && known[target] != nil {
					text = referenceCell(text, target, known[target])
				} else {
					text = tableCellText(text)
				}
				b.WriteString(" " + text + " |")
			}
			b.WriteString("\n")
		}
		return raymond.SafeString(b.String())
	})
}

func referenceCell(s, target string, known map[string]bool) string {
	codes := template.SplitReferenceCodes(s)
	parts := make([]string, 0, len(codes))
	for _, code := range codes {
		text := tableCellText(code)
		if id := template.ReferenceAnchor(target, code); id != "" && known[strings.ToLower(code)] {
			text = "[" + text + "](#" + id + ")"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ", ")
}
