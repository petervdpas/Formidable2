package render

import (
	"fmt"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// RenderMarkdown runs the Handlebars stage. It composes a context
// `{...values, _fields, _template, _loopGroups}`, registers Formidable's
// helper set with a fresh per-call scratch (setVar/getVar), and executes
// tpl.MarkdownTemplate. An empty MarkdownTemplate returns a placeholder
// (mirrors the original "No template defined." sentinel).
func RenderMarkdown(values map[string]any, tpl *template.Template, opts *Options) (string, error) {
	if tpl == nil || tpl.MarkdownTemplate == "" {
		return "# No template defined.", nil
	}
	if values == nil {
		values = map[string]any{}
	}
	if opts == nil {
		opts = &Options{}
	}

	parsed, err := raymond.Parse(tpl.MarkdownTemplate)
	if err != nil {
		return "", fmt.Errorf("render: parse markdown template: %w", err)
	}
	registerHelpers(parsed, opts, map[string]any{}, tpl.Fields)

	ctx := make(map[string]any, len(values)+3)
	for k, v := range values {
		ctx[k] = v
	}
	ctx["_fields"] = tpl.Fields
	ctx["_template"] = tpl
	ctx["_loopGroups"] = buildNestedLoopGroups(tpl.Fields)

	out, err := parsed.Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("render: exec markdown template: %w", err)
	}
	return out, nil
}
