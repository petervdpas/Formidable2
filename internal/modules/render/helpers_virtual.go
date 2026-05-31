package render

import (
	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// registerVirtualFieldHelper binds {{virtual-field "fieldKey"}}, the
// render-time projection of a virtual (data-less) field. The argument is
// the field's template key, so the call shape generalises to any future
// virtual type. Today the only type is `facet`: it reads FacetKey and
// returns the selected label from opts.Facets. Unknown or non-virtual
// keys return empty (fail-safe; use {{field}} for non-virtual fields).
func registerVirtualFieldHelper(tpl *raymond.Template, opts *Options, rootFields []template.Field) {
	tpl.RegisterHelper("virtual-field", func(options *raymond.Options) string {
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
		}
		return ""
	})
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
