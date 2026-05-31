package wiki

import (
	"html/template"
	"strings"

	tpl "github.com/petervdpas/formidable2/internal/modules/template"
)

// facetIconSVG renders one FA glyph as inline SVG; fill="currentColor" lets the chip's text colour drive the fill.
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
