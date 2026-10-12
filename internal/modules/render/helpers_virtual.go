package render

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/diagram"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// registerVirtualFieldHelper binds {{virtual-field "fieldKey"}}, the
// render-time projection of a virtual (data-less) field. The argument is
// the field's template key, so the call shape generalises to any future
// virtual type. `facet` returns the selected label from opts.Facets;
// `diagram` references the record's generated diagram image (or inlines the SVG). Unknown or
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
			return raymond.SafeString(emitDiagram(options, opts, rootFields, key))
		}
		return ""
	})
}

// emitDiagram renders a diagram field as its SVG, wrapped in a div so
// CommonMark treats it as one HTML block. In "image" mode (second positional
// arg or `mode=`) the SVG is instead handed to opts.DiagramImage to be kept as
// an image file and, when that succeeds, referenced as a markdown image
// resolved like any image field; without a record, a writer or a successful
// write it stays inline, so a reference never dangles. It reads the root
// record, so it is meant for root context, not inside a loop.
func emitDiagram(options *raymond.Options, opts *Options, rootFields []template.Field, key string) string {
	ctx := contextMap(options.Ctx())
	t, _ := ctx["_template"].(*template.Template)
	if t == nil {
		t = &template.Template{Fields: rootFields}
	}
	svg, ok := diagram.RenderField(t, key, ctx)
	if !ok {
		return ""
	}
	name := ""
	if opts != nil && opts.DiagramImage != nil && virtualFieldMode(options) == "image" {
		name = template.DiagramImageName(opts.TemplateFilename, opts.Datafile, key)
	}
	stored := name != "" && opts.DiagramImage(name, svg)
	if svg == "" {
		return ""
	}
	if !stored {
		return "<div class=\"formidable-diagram\">\n" + svg + "\n</div>"
	}
	alt := key
	if f := findFieldByKey(rootFields, key); f != nil && f.Label != "" {
		alt = strings.NewReplacer("[", "", "]", "").Replace(f.Label)
	}
	url := emitImage(name, opts)
	if opts.diagramImages == nil {
		opts.diagramImages = map[string]string{}
	}
	sum := sha256.Sum256([]byte(svg))
	opts.diagramImages[url] = hex.EncodeToString(sum[:4])
	return "![" + alt + "](" + url + ")"
}

// virtualFieldMode reads {{virtual-field "key" "mode"}}; the hash form
// `mode=` wins, as on {{field}}.
func virtualFieldMode(options *raymond.Options) string {
	mode := ""
	if p := options.Params(); len(p) > 1 {
		mode, _ = p[1].(string)
	}
	if h := options.HashStr("mode"); h != "" {
		mode = h
	}
	return strings.ToLower(strings.TrimSpace(mode))
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
