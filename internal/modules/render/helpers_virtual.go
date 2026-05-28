package render

import (
	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// registerVirtualFieldHelper binds {{virtual-field "fieldKey"}}, the
// render-time projection of any virtual (data-less) field. The
// argument is the field's key on the template, NOT the underlying
// projection key, so the same call shape works for any future
// virtual type: the helper finds the field by key, dispatches on
// its declared type, and emits a string.
//
// Today the only virtual type is `facet`: the helper reads the
// field's FacetKey, looks up the selected label in opts.Facets, and
// returns it (empty when unset). Unknown / non-virtual field keys
// also return empty - the helper fails safe rather than leaking the
// data-side value (use {{field "k"}} for non-virtual fields).
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

// findFieldByKey scans the template's root field list for the named
// key. Loop-body fields aren't candidates - virtual fields never
// live inside loops (the registry has no loop-typed virtual entry
// today), and {{virtual-field}} reads from the root context where
// rootFields lives.
func findFieldByKey(fields []template.Field, key string) *template.Field {
	for i := range fields {
		if fields[i].Key == key {
			return &fields[i]
		}
	}
	return nil
}
