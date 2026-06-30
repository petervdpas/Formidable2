package render

import (
	"strings"

	"github.com/aymerick/raymond"
)

// registerFieldHelper binds the polymorphic {{field}} helper, supporting
// both `{{field "key"}}` and `{{field "key" "mode"}}`. Raymond's strict
// arity rejects the 2-positional call when the helper declares typed
// params, so it registers as options-only (see third_party/raymond/
// CHANGES.md "options-only variadic helpers") and reads positional args
// itself.
func registerFieldHelper(tpl *raymond.Template, opts *Options) {
	tpl.RegisterHelper("field", func(options *raymond.Options) any {
		params := options.Params()
		var key, mode string
		modeExplicit := false
		if len(params) > 0 {
			key, _ = params[0].(string)
		}
		if len(params) > 1 {
			if s, ok := params[1].(string); ok && s != "" {
				mode = s
				modeExplicit = true
			}
		}
		// hash form `mode=` wins over positional, matching JS precedence.
		if h := options.HashStr("mode"); h != "" {
			mode = h
			modeExplicit = true
		}
		mode = strings.ToLower(mode)
		// Default mode is type-dependent: link fields default to
		// "default" (Markdown link), everything else to "label". JS
		// arity passed the options hash into the mode slot, so bare
		// `{{field "k"}}` link calls fell through to the link branch; we
		// reproduce that intentional accident so bare calls stay links
		// while explicit `mode="label"` gives label-only.

		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		field := findField(options.Ctx(), key)
		if field == nil {
			return "(unknown field: " + key + ")"
		}
		value := ctx[key]

		if mode == "" {
			if field.Type == "link" {
				mode = "default"
			} else {
				mode = "label"
			}
		}
		_ = modeExplicit // reserved for future per-field defaults

		switch field.Type {
		case "multioption":
			arr, ok := value.([]any)
			if !ok {
				return ""
			}
			out := make([]string, 0, len(arr))
			for _, item := range arr {
				if mode == "value" {
					out = append(out, stringify(item))
				} else {
					out = append(out, emitOptionLabel(item, field))
				}
			}
			return strings.Join(out, ", ")
		case "link":
			href, text := linkParts(value)
			href = resolveLinkHref(href, opts)
			switch mode {
			case "href", "value":
				return href
			case "text":
				if text != "" {
					return text
				}
				return href
			case "label":
				if text != "" {
					return text
				}
				return href
			default:
				if href == "" {
					return text
				}
				label := text
				if label == "" {
					label = href
				}
				return raymond.SafeString("[" + label + "](" + href + ")")
			}
		case "dropdown", "radio", "table":
			if mode == "value" {
				return stringify(value)
			}
			return emitOptionLabel(value, field)
		case "textarea":
			return raymond.SafeString(stringify(value))
		case "mermaid":
			return raymond.SafeString(emitMermaid(value))
		case "slide":
			return raymond.SafeString(renderSlide(value, opts))
		case "image":
			return emitImage(value, opts)
		}
		return emitFieldValue(value, field, opts)
	})
}
