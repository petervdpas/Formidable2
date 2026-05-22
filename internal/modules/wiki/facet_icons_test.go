package wiki

import (
	"strings"
	"testing"

	tpl "github.com/petervdpas/formidable2/internal/modules/template"
)

// facetIconSVG must never emit the xmlns attribute - the user
// explicitly asked for inline-SVG-only with no URL-shaped tokens in
// the output. This is a regression guard.
func TestFacetIconSVG_NoXMLNSAttribute(t *testing.T) {
	for _, key := range tpl.FacetIconList {
		got := string(facetIconSVG(key))
		if strings.Contains(got, "xmlns") {
			t.Errorf("icon %q emitted an xmlns attr: %s", key, got)
		}
	}
}

// Unknown icon keys fall back to fa-flag rather than rendering an
// empty <svg/> - keeps stale templates from disappearing into thin air.
func TestFacetIconSVG_UnknownFallsBackToFlag(t *testing.T) {
	want := string(facetIconSVG("fa-flag"))
	for _, key := range []string{"", "fa-unknown", "nonsense"} {
		got := string(facetIconSVG(key))
		if got != want {
			t.Errorf("icon %q did not fall back to fa-flag\n got: %s\nwant: %s", key, got, want)
		}
	}
}

// Sanity: the emitted SVG carries the path data and currentColor fill
// hook so CSS can drive the colour from the surrounding chip's text.
func TestFacetIconSVG_EmitsPathWithCurrentColor(t *testing.T) {
	got := string(facetIconSVG("fa-shirt"))
	for _, want := range []string{
		`viewBox="0 0 640 512"`,
		`fill="currentColor"`,
		`class="facet-icon-svg"`,
		`<path`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("svg missing %q\n full: %s", want, got)
		}
	}
}
