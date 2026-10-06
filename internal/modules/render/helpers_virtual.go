package render

import (
	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/diagram"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// registerVirtualFieldHelper binds {{virtual-field "fieldKey"}}, the
// render-time projection of a virtual (data-less) field. The argument is
// the field's template key, so the call shape generalises to any future
// virtual type. `facet` returns the selected label from opts.Facets;
// `diagram` draws its bound root table as an inline SVG block. Unknown or
// non-virtual keys return empty (fail-safe; use {{field}} for non-virtual fields).
func registerVirtualFieldHelper(tpl *raymond.Template, opts *Options, rootFields []template.Field) {
	tpl.RegisterHelper("virtual-field", func(options *raymond.Options) any {
		params := options.Params()
		if len(params) == 0 {
			return ""
		}
		key, _ := params[0].(string)
		if key == "" {
			return ""
		}
		f := findFieldByKey(rootFields, key)
		if f == nil {
			return ""
		}
		switch f.Type {
		case "facet":
			if opts == nil || opts.Facets == nil {
				return ""
			}
			return opts.Facets[f.FacetKey]
		case "diagram":
			return raymond.SafeString(emitDiagram(options, rootFields, key))
		}
		return ""
	})
}

// emitDiagram wraps the SVG in a div so CommonMark treats it as one HTML
// block (no <p> wrapper) on the markdown and HTML/PDF paths alike. It reads
// the root record, so it is meant for root context, not inside a loop.
func emitDiagram(options *raymond.Options, rootFields []template.Field, key string) string {
	ctx := contextMap(options.Ctx())
	t, _ := ctx["_template"].(*template.Template)
	if t == nil {
		t = &template.Template{Fields: rootFields}
	}
	svg, ok := diagram.RenderField(t, key, ctx)
	if !ok || svg == "" {
		return ""
	}
	return "<div class=\"formidable-diagram\">\n" + svg + "\n</div>"
}

// findFieldByKey scans the root field list only; virtual fields never
// live inside loops, and {{virtual-field}} reads from the root context.
func findFieldByKey(fields []template.Field, key string) *template.Field {
	for i := range fields {
		if fields[i].Key == key {
			return &fields[i]
		}
	}
	return nil
}
