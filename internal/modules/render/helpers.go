package render

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// registerHelpers binds every Handlebars helper Formidable's render
// pipeline ships with. opts carries URL strategies; vars is a scratch
// map for setVar/getVar — fresh per RenderMarkdown call so renders
// can't leak state into each other (original JS used a module-level
// store; that bug isn't ported).
func registerHelpers(tpl *raymond.Template, opts *Options, vars map[string]any) {
	if opts == nil {
		opts = &Options{}
	}
	if vars == nil {
		vars = map[string]any{}
	}

	// ── data helpers ──────────────────────────────────────────────
	tpl.RegisterHelper("json", func(value any) raymond.SafeString {
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return raymond.SafeString("")
		}
		return raymond.SafeString(b)
	})
	tpl.RegisterHelper("log", func(value any) string {
		b, _ := json.MarshalIndent(value, "", "  ")
		return "\n[LOG] " + string(b) + "\n"
	})

	// ── comparison helpers ────────────────────────────────────────
	cmpOps := map[string]string{
		"eq": "===", "ne": "!==", "lt": "<", "lte": "<=", "gt": ">", "gte": ">=",
	}
	for name, op := range cmpOps {
		op := op
		tpl.RegisterHelper(name, func(a, b any) bool {
			return Compare(a, op, b)
		})
	}

	// ── math helpers ─────────────────────────────────────────────
	mathOps := []string{
		"add", "subtract", "multiply", "divide", "mod",
		"pad", "abs", "round", "ceil", "floor",
	}
	for _, name := range mathOps {
		op := name
		tpl.RegisterHelper(name, func(a, b any) string {
			return stringify(EvaluateMath(a, op, b))
		})
	}
	tpl.RegisterHelper("math", func(a any, op string, b any) string {
		return stringify(EvaluateMath(a, op, b))
	})
	tpl.RegisterHelper("compare", func(a any, op string, b any) bool {
		return Compare(a, op, b)
	})

	// ── array / collection helpers ───────────────────────────────
	tpl.RegisterHelper("length", func(arr any) int {
		v := reflect.ValueOf(arr)
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			return v.Len()
		}
		return 0
	})
	tpl.RegisterHelper("includes", func(arr any, value any) bool {
		return arrayIncludes(arr, value)
	})
	tpl.RegisterHelper("isSelected", func(arr, value any, options *raymond.Options) string {
		if arrayIncludes(arr, value) {
			return options.Fn()
		}
		return options.Inverse()
	})

	// ── string helpers ───────────────────────────────────────────
	tpl.RegisterHelper("pascal", func(s string) string {
		if s == "" {
			return ""
		}
		return strings.ToUpper(s[:1]) + s[1:]
	})
	tpl.RegisterHelper("camel", func(s string) string {
		if s == "" {
			return ""
		}
		return strings.ToLower(s[:1]) + s[1:]
	})

	// ── per-call scratch vars ────────────────────────────────────
	tpl.RegisterHelper("setVar", func(name string, value any) string {
		vars[name] = value
		return ""
	})
	tpl.RegisterHelper("getVar", func(name string) string {
		return stringify(vars[name])
	})

	// ── option lookup ────────────────────────────────────────────
	tpl.RegisterHelper("lookupOption", func(options any, value any) map[string]any {
		want := stringify(value)
		arr, ok := options.([]any)
		if !ok {
			return map[string]any{"value": want, "label": want}
		}
		for _, opt := range arr {
			val, lab := optionPair(opt)
			if val == want {
				return map[string]any{"value": val, "label": lab}
			}
		}
		return map[string]any{"value": want, "label": want}
	})

	// ── table cell ───────────────────────────────────────────────
	tpl.RegisterHelper("cell", func(row any, colName, tableKey string, options *raymond.Options) string {
		fields := contextFields(options.Ctx())
		var tableField *template.Field
		for i := range fields {
			if fields[i].Key == tableKey {
				tableField = &fields[i]
				break
			}
		}
		if tableField == nil || len(tableField.Options) == 0 {
			return ""
		}
		idx := -1
		for i, opt := range tableField.Options {
			val, _ := optionPair(opt)
			if val == colName {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ""
		}
		cells, ok := row.([]any)
		if !ok || idx >= len(cells) {
			return ""
		}
		return stringify(cells[idx])
	})

	// ── field accessors ──────────────────────────────────────────
	tpl.RegisterHelper("fieldRaw", func(key string, options *raymond.Options) any {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		return ctx[key]
	})
	tpl.RegisterHelper("fieldMeta", func(key, prop string, options *raymond.Options) any {
		f := findField(options.Ctx(), key)
		if f == nil {
			return ""
		}
		switch prop {
		case "":
			return f
		case "key":
			return f.Key
		case "type":
			return f.Type
		case "label":
			return f.Label
		case "description":
			return f.Description
		case "options":
			return f.Options
		default:
			return ""
		}
	})
	// imageURL resolves a field's stored filename to its target URL via
	// Options.ImageURL. The slideout's imageURLFunc returns
	// `/api/images/<stem>/<file>`, the wiki's returns `/storage/<stem>/
	// images/<file>` — same helper, different transport. Returns "" for
	// unknown fields or empty values; falls back to "images/<name>"
	// when no ImageURL func is wired (matches emitImage's defaults).
	tpl.RegisterHelper("imageURL", func(key string, options *raymond.Options) string {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		name, ok := ctx[key].(string)
		if !ok || name == "" {
			return ""
		}
		if isAbsoluteURL(name) || strings.HasPrefix(name, "file:") {
			return name
		}
		if opts != nil && opts.ImageURL != nil {
			return opts.ImageURL(name)
		}
		return "images/" + name
	})

	// imageBase64 resolves a field's stored filename to a
	// `data:<mime>;base64,<bytes>` URL via Options.ImageBase64URL. Used
	// by the generator's "inline" image mode for self-contained
	// markdown exports. Returns "" when the func isn't wired or the
	// field value is empty — distinct from imageURL's `images/<name>`
	// fallback because there's no sensible default for an inlined byte
	// stream.
	tpl.RegisterHelper("imageBase64", func(key string, options *raymond.Options) string {
		if opts == nil || opts.ImageBase64URL == nil {
			return ""
		}
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		name, ok := ctx[key].(string)
		if !ok || name == "" {
			return ""
		}
		return opts.ImageBase64URL(name)
	})

	tpl.RegisterHelper("fieldDescription", func(key string, options *raymond.Options) string {
		f := findField(options.Ctx(), key)
		if f == nil {
			return ""
		}
		return f.Description
	})

	// {{field}} — the big polymorphic dispatch helper.
	// Implementation lives in helpers_field.go.
	registerFieldHelper(tpl, opts)

	// ── loop block helper ───────────────────────────────────────
	//
	// Plain iterator — same shape as the original Formidable's helper.
	// Iteration wrapping is opt-in: place {{loopItemBefore}} and
	// {{loopItemAfter}} inside the body to wrap each iteration in
	// `<section class="loop-item" …>`. The generator emits those calls
	// when the user toggles "Wrap loop iterations" on.
	//
	// Each iteration's context carries `_loopKey` and `_loopIndex` so
	// the before/after helpers (and {{loopKey}} / {{loopIndex}}) can
	// read them without scanning.
	tpl.RegisterHelper("loop", func(key string, options *raymond.Options) string {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		items, ok := ctx[key].([]any)
		if !ok {
			return ""
		}
		groupFields := loopGroupFields(ctx, key)
		// Append a synthetic <key>_index so {{field "<key>_index"}} works.
		syntheticIndex := template.Field{
			Key:         key + "_index",
			Type:        "number",
			Label:       key + " index",
			Description: "Auto-generated index for loop \"" + key + "\"",
		}
		combined := append([]template.Field(nil), groupFields...)
		combined = append(combined, syntheticIndex)

		tplPtr, _ := ctx["_template"].(*template.Template)
		groups, _ := ctx["_loopGroups"].(map[string][]template.Field)

		out := make([]string, 0, len(items))
		for i, raw := range items {
			entry, _ := raw.(map[string]any)
			sub := make(map[string]any, len(entry)+6)
			for k, v := range entry {
				sub[k] = v
			}
			sub[key+"_index"] = i + 1
			sub["_loopKey"] = key
			sub["_loopIndex"] = i + 1
			sub["_fields"] = combined
			sub["_template"] = tplPtr
			sub["_loopGroups"] = groups
			out = append(out, options.FnWith(sub))
		}
		return strings.Join(out, "\n")
	})

	// {{loopItemBefore [extra-classes…]}} — emits the section opener
	// for the current iteration. Variadic extras are appended after
	// the base "loop-item" class, so the user can theme individual
	// loops without touching the helper.
	//
	// Outside a loop body (no _loopKey) → empty string.
	tpl.RegisterHelper("loopItemBefore", func(options *raymond.Options) raymond.SafeString {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		key, _ := ctx["_loopKey"].(string)
		if key == "" {
			return ""
		}
		idx := loopIndexFromCtx(ctx)
		extras := []string{}
		for _, p := range options.Params() {
			if s, ok := p.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					extras = append(extras, s)
				}
			}
		}
		classAttr := "loop-item"
		if len(extras) > 0 {
			classAttr += " " + strings.Join(extras, " ")
		}
		return raymond.SafeString(fmt.Sprintf(
			"<section class=%q data-loop=%q data-index=%q>\n\n",
			classAttr, key, strconv.Itoa(idx),
		))
	})

	// {{loopItemAfter}} — pairs with {{loopItemBefore}}. The leading
	// blank line is what tells goldmark to close the HTML block above
	// and resume markdown parsing. Outside a loop → empty.
	tpl.RegisterHelper("loopItemAfter", func(options *raymond.Options) raymond.SafeString {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		key, _ := ctx["_loopKey"].(string)
		if key == "" {
			return ""
		}
		return raymond.SafeString("\n\n</section>")
	})

	// {{loopKey}} — current loop's key. Empty outside a loop body.
	tpl.RegisterHelper("loopKey", func(options *raymond.Options) string {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		k, _ := ctx["_loopKey"].(string)
		return k
	})

	// {{loopIndex}} — current iteration's 1-based index. 0 outside.
	tpl.RegisterHelper("loopIndex", func(options *raymond.Options) string {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		switch v := ctx["_loopIndex"].(type) {
		case int:
			return strconv.Itoa(v)
		case int64:
			return strconv.FormatInt(v, 10)
		case float64:
			return strconv.Itoa(int(v))
		}
		return ""
	})

	// {{loopItemClass [extra1] [extra2] …}} — variadic class composer.
	// Always emits "loop-item" as the base; each non-empty extra arg is
	// appended space-separated. Used inside `wrap=false` bodies where
	// the user builds their own <article>/<div>/etc. wrapper.
	tpl.RegisterHelper("loopItemClass", func(options *raymond.Options) string {
		parts := []string{"loop-item"}
		for _, p := range options.Params() {
			s, _ := p.(string)
			s = strings.TrimSpace(s)
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	})

	// `stats` is polymorphic: `{{stats t}}` (colIndex defaults to 1) and
	// `{{stats t 2}}` are both valid in the original JS. Options-only
	// signature so we can default colIndex when absent.
	tpl.RegisterHelper("stats", func(options *raymond.Options) string {
		params := options.Params()
		var table any
		colIndex := 1
		if len(params) > 0 {
			table = params[0]
		}
		if len(params) > 1 {
			if i, ok := params[1].(int); ok {
				colIndex = i
			} else {
				colIndex = int(toFloat(params[1]))
			}
		}
		rows, ok := table.([]any)
		if !ok {
			return "_no data_"
		}
		values := make([]any, 0, len(rows))
		for _, r := range rows {
			cells, ok := r.([]any)
			if !ok || colIndex >= len(cells) {
				continue
			}
			values = append(values, cells[colIndex])
		}
		var pp *float64
		if raw := options.HashProp("percentile"); raw != nil {
			f := toFloat(raw)
			pp = &f
		}
		s := ComputeStats(values, pp)
		if s == nil {
			return "_no data_"
		}
		parts := []string{
			"min=" + stringify(s.Min),
			"max=" + stringify(s.Max),
			"avg=" + strconv.FormatFloat(s.Avg, 'f', 2, 64),
			"median=" + stringify(s.Median),
			"stddev=" + strconv.FormatFloat(s.Stddev, 'f', 2, 64),
		}
		if s.Percentile != nil && s.PercentileInput != nil {
			parts = append(parts,
				"p"+strconv.FormatFloat(*s.PercentileInput, 'f', -1, 64)+
					"="+strconv.FormatFloat(*s.Percentile, 'f', 2, 64))
		}
		return strings.Join(parts, ", ")
	})

	// ── tags ─────────────────────────────────────────────────────
	// `tags` is polymorphic: `{{tags}}` (default array []) and
	// `{{tags arr withHash=true}}` are both valid in the original JS.
	// Options-only signature lets us read positional args manually
	// (see third_party/raymond/CHANGES.md "options-only variadic").
	tpl.RegisterHelper("tags", func(options *raymond.Options) string {
		var arr any
		if params := options.Params(); len(params) > 0 {
			arr = params[0]
		}
		withHash := true
		if raw := options.HashProp("withHash"); raw != nil {
			if b, ok := raw.(bool); ok {
				withHash = b
			}
		}
		return emitTags(arr, withHash)
	})

	// {{apiCol}} / {{apiBlock}} / {{apiGuid}} / {{apiSection}}.
	// Implementations live in apifield_helpers.go.
	registerAPIFieldHelpers(tpl, opts)
}

// linkParts unpacks string or {href,text} into (href, text).
func linkParts(v any) (string, string) {
	switch x := v.(type) {
	case string:
		return x, ""
	case map[string]any:
		return stringify(x["href"]), stringify(x["text"])
	}
	return "", ""
}

// arrayIncludes checks slice membership with stringy equality so
// `{1,2,3}.includes("2")` works the same way the JS helper did.
func arrayIncludes(arr, value any) bool {
	v := reflect.ValueOf(arr)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return false
	}
	want := fmt.Sprint(value)
	for i := 0; i < v.Len(); i++ {
		if fmt.Sprint(v.Index(i).Interface()) == want {
			return true
		}
	}
	return false
}

