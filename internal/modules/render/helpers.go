package render

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// nowFn is the clock used by {{today}} / {{now}}; overridden in tests.
var nowFn = time.Now

// registerHelpers binds every Handlebars helper. vars is a scratch map
// for setVar/getVar, fresh per RenderMarkdown call so renders can't leak
// state into each other (the original JS used a leaky module-level store).
func registerHelpers(tpl *raymond.Template, opts *Options, vars map[string]any, rootFields []template.Field) {
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
		if len(fields) == 0 {
			fields = rootFields
		}
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
		v := ctx[key]
		// A list field's items may be {text,indent} objects (indented rows). Expose
		// the flat texts so {{#each (fieldRaw "k")}}{{this}} keeps rendering strings
		// (the list reads as unindented here); indent nesting is {{field}}'s job.
		if f := findField(options.Ctx(), key); f != nil && f.Type == "list" {
			if arr, ok := v.([]any); ok {
				out := make([]any, len(arr))
				for i, item := range arr {
					out[i] = template.ListItemText(item)
				}
				return out
			}
		}
		return v
	})
	// {{list "key" ordered=true}} (or mode="ordered") renders a list field as
	// Markdown, bulleted by default or numbered when ordered; indented rows nest
	// either way. Note Handlebars treats a bare `ordered` as a variable, so the
	// string form must be quoted (`mode="ordered"`); the boolean `ordered=true` is
	// the idiomatic unquoted flag.
	tpl.RegisterHelper("list", func(key string, options *raymond.Options) string {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		ordered := strings.EqualFold(options.HashStr("mode"), "ordered")
		if raw := options.HashProp("ordered"); raw != nil {
			if b, ok := raw.(bool); ok && b {
				ordered = true
			}
		}
		return emitListMode(ctx[key], ordered)
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
			// api fields carry columns in Map, not Options; expose them in the
			// same {value,label} shape so the table header idiom works verbatim:
			// {{#each (fieldMeta "apiKey" "options")}}{{label}}{{/each}}.
			if f.Type == "api" {
				return apiColumnOptions(f)
			}
			return f.Options
		default:
			return ""
		}
	})
	// imageURL resolves a field's filename to its target URL via
	// Options.ImageURL (slideout and wiki wire different transports). It
	// falls back to "images/<name>" when no ImageURL func is wired.
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

	// imageBase64 resolves a field's filename to a `data:...base64,...`
	// URL via Options.ImageBase64URL, for the generator's inline image
	// mode. It returns "" (rather than imageURL's `images/<name>`
	// fallback) when unwired, since an inlined byte stream has no default.
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

	// mermaid emits a mermaid field's diagram source as a fenced
	// ```mermaid block - the dedicated accessor for the mermaid field
	// type, mirroring imageURL/imageBase64 for image fields. Same output
	// as `{{field "key"}}` on a mermaid field, but discoverable by name.
	// Empty / missing source emits nothing (no stray empty fence).
	tpl.RegisterHelper("mermaid", func(key string, options *raymond.Options) any {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		return raymond.SafeString(emitMermaid(ctx[key]))
	})

	// {{field}}: implementation in helpers_field.go.
	registerFieldHelper(tpl, opts)

	// {{board}}: plan-board render (mermaid Gantt + events table).
	registerBoardHelper(tpl, opts)

	// {{boardMeta "prop"}}: read one scalar off the plan-board record.
	registerBoardMetaHelper(tpl)

	// {{#boardSlices}}: iterate calendar slices of the plan board (Gantt + table
	// per page). Implementation in boardrender.go.
	registerBoardSlicesHelper(tpl, opts)

	// {{virtual-field "key"}}: implementation in helpers_virtual.go.
	registerVirtualFieldHelper(tpl, opts, rootFields)

	// ── meta helpers ────────────────────────────────────────────
	//
	// Identity of the (template, datafile) pair being rendered, for
	// composing anchors/slugs/filenames in export paths. Empty Options
	// yields empty strings so templates stay safe when meta isn't wired.
	tpl.RegisterHelper("templateName", func() string {
		return opts.TemplateFilename
	})
	tpl.RegisterHelper("templateStem", func() string {
		return strings.TrimSuffix(opts.TemplateFilename, ".yaml")
	})
	tpl.RegisterHelper("datafile", func() string {
		return opts.Datafile
	})
	tpl.RegisterHelper("datafileStem", func() string {
		return strings.TrimSuffix(opts.Datafile, ".meta.json")
	})

	// ── loop block helper ───────────────────────────────────────
	//
	// Plain iterator. Wrapping is opt-in: place {{loopItemBefore}} /
	// {{loopItemAfter}} in the body to wrap each iteration in a
	// `<section class="loop-item">`. Each iteration's context carries
	// `_loopKey` and `_loopIndex` for the before/after and key/index
	// helpers to read.
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
		// Synthetic <key>_index so {{field "<key>_index"}} works.
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

	// {{loopItemBefore [extra-classes...]}} emits the iteration's section
	// opener; variadic extras append after the base "loop-item" class.
	// Empty outside a loop body.
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

	// {{loopItemAfter}} pairs with {{loopItemBefore}}. The leading blank
	// line is what tells goldmark to close the HTML block above and resume
	// markdown parsing. Empty outside a loop.
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

	// {{loopKey}}: current loop's key, empty outside a loop body.
	tpl.RegisterHelper("loopKey", func(options *raymond.Options) string {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		k, _ := ctx["_loopKey"].(string)
		return k
	})

	// {{loopIndex}}: current iteration's 1-based index, 0 outside.
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

	// {{today}}: current date as YYYY-MM-DD. For other layouts use
	// {{now "FORMAT"}} with a Go time layout string.
	tpl.RegisterHelper("today", func() string {
		return nowFn().Format("2006-01-02")
	})

	// {{now [layout] [locale]}}: current time in a Go time layout
	// (default "2006-01-02 15:04:05"), optionally translated into a
	// locale (en/nl/de/fr; see locale.go to add more).
	tpl.RegisterHelper("now", func(options *raymond.Options) string {
		layout := "2006-01-02 15:04:05"
		locale := ""
		params := options.Params()
		if len(params) >= 1 {
			if s, ok := params[0].(string); ok && strings.TrimSpace(s) != "" {
				layout = s
			}
		}
		if len(params) >= 2 {
			if s, ok := params[1].(string); ok {
				locale = s
			}
		}
		return translateDate(nowFn().Format(layout), locale)
	})

	// {{dateFormat value layout [locale]}}: reformat a stored date with
	// a Go time layout, optionally locale-translated. Accepts 2006-01-02,
	// RFC 3339, and "2006-01-02 15:04:05". Unparseable input is returned
	// verbatim so the template doesn't silently lose data.
	tpl.RegisterHelper("dateFormat", func(options *raymond.Options) string {
		params := options.Params()
		if len(params) < 2 {
			return ""
		}
		s := strings.TrimSpace(stringify(params[0]))
		if s == "" {
			return ""
		}
		layout, _ := params[1].(string)
		if layout == "" {
			return s
		}
		locale := ""
		if len(params) >= 3 {
			if l, ok := params[2].(string); ok {
				locale = l
			}
		}
		inputs := []string{
			"2006-01-02",
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
		}
		for _, l := range inputs {
			if t, err := time.Parse(l, s); err == nil {
				return translateDate(t.Format(layout), locale)
			}
		}
		return s
	})

	// {{loopItemClass [extra...]}}: composes a class string with
	// "loop-item" as the base, for bodies that build their own wrapper.
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

	// stats is polymorphic (`{{stats t}}` defaults colIndex to 1,
	// `{{stats t 2}}` sets it). Options-only signature defaults colIndex
	// when absent.
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
	// tags is polymorphic (`{{tags}}` or `{{tags arr withHash=true}}`).
	// Options-only signature reads positional args manually (see
	// third_party/raymond/CHANGES.md "options-only variadic").
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

	// ── yamlList ─────────────────────────────────────────────────
	// {{yamlList arr}} emits a YAML block sequence, one `- item` per
	// element, items 2+ optionally indent=N-padded for a non-zero column.
	// Built for the PDF `keywords:` migration so tags expand to real list
	// items rather than a comma-blob.
	tpl.RegisterHelper("yamlList", func(options *raymond.Options) raymond.SafeString {
		var arr any
		if params := options.Params(); len(params) > 0 {
			arr = params[0]
		}
		indent := 0
		if raw := options.HashProp("indent"); raw != nil {
			switch v := raw.(type) {
			case int:
				indent = v
			case int64:
				indent = int(v)
			case float64:
				indent = int(v)
			}
		}
		return raymond.SafeString(emitYAMLList(arr, indent))
	})

	// yamlString encodes a scalar as a quoted YAML value for frontmatter, as a
	// SafeString so raymond doesn't HTML-escape it (`&` -> `&amp;`). The YAML
	// counterpart to yamlList for single values like `title:`.
	tpl.RegisterHelper("yamlString", func(options *raymond.Options) raymond.SafeString {
		var v any
		if params := options.Params(); len(params) > 0 {
			v = params[0]
		}
		return raymond.SafeString(emitYAMLString(v))
	})

	// api field helpers: implementations in apifield_helpers.go.
	registerAPIFieldHelpers(tpl, opts)
}

func linkParts(v any) (string, string) {
	switch x := v.(type) {
	case string:
		return x, ""
	case map[string]any:
		return stringify(x["href"]), stringify(x["text"])
	}
	return "", ""
}

// arrayIncludes checks slice membership with stringy equality, so
// `[1,2,3]` includes "2" as the JS helper did.
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

