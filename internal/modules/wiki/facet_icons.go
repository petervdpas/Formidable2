package wiki

import (
	"html/template"
	"strings"

	tpl "github.com/petervdpas/formidable2/internal/modules/template"
)

// facetIconSVG renders one FA glyph as inline SVG using the shared
// catalog that lives next to FacetIconList in the template module.
// Unknown / empty keys fall back to fa-flag (template.FacetIconSpecFor's
// contract). Output uses `fill="currentColor"` so the surrounding
// chip's text colour drives the glyph fill.
func facetIconSVG(icon string) template.HTML {
	spec := tpl.FacetIconSpecFor(icon)
	var b strings.Builder
	b.WriteString(`<svg class="facet-icon-svg" viewBox="`)
	b.WriteString(spec.ViewBox)
	b.WriteString(`" aria-hidden="true"><path fill="currentColor" d="`)
	b.WriteString(spec.Path)
	b.WriteString(`"/></svg>`)
	return template.HTML(b.String())
}
