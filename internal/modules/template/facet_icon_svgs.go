package template

import (
	"embed"
	"fmt"
	iofs "io/fs"
	"regexp"
	"strings"
)

// FacetIconSpec carries the viewBox + path d for one inline-SVG glyph; no xmlns/fill (each layer wraps its own shell).
type FacetIconSpec struct {
	ViewBox string `json:"viewBox"`
	Path    string `json:"path"`
}

// FacetIconSVGs is the parsed catalog keyed by the "fa-" form, built at init() from icons/*.svg;
// missing files panic at startup so a missing glyph never ships silently.
var FacetIconSVGs map[string]FacetIconSpec

//go:embed icons/*.svg
var facetIconFS embed.FS

var (
	reIconViewBox = regexp.MustCompile(`viewBox="([^"]+)"`)
	reIconPath    = regexp.MustCompile(`<path[^>]*\bd="([^"]+)"`)
)

func init() {
	FacetIconSVGs = make(map[string]FacetIconSpec, len(FacetIconList))
	for _, key := range FacetIconList {
		name := strings.TrimPrefix(key, "fa-") + ".svg"
		raw, err := iofs.ReadFile(facetIconFS, "icons/"+name)
		if err != nil {
			panic(fmt.Sprintf("template: facet icon %q (file %q) missing from embed: %v", key, name, err))
		}
		spec, err := parseFacetIconSVG(raw)
		if err != nil {
			panic(fmt.Sprintf("template: facet icon %q (file %q) parse failed: %v", key, name, err))
		}
		FacetIconSVGs[key] = spec
	}
}

// parseFacetIconSVG pulls the viewBox and first <path d="..."/> from an SVG; the closed FA palette
// is always single-path with no gradients, so regex suffices over a real parser.
func parseFacetIconSVG(raw []byte) (FacetIconSpec, error) {
	src := string(raw)
	vb := reIconViewBox.FindStringSubmatch(src)
	if len(vb) != 2 {
		return FacetIconSpec{}, fmt.Errorf("no viewBox attribute")
	}
	pd := reIconPath.FindStringSubmatch(src)
	if len(pd) != 2 {
		return FacetIconSpec{}, fmt.Errorf("no <path d=\"…\"/> element")
	}
	return FacetIconSpec{ViewBox: vb[1], Path: pd[1]}, nil
}

// FacetIconSpecFor returns the spec for key, falling back to fa-flag so a stale reference still renders a glyph.
func FacetIconSpecFor(key string) FacetIconSpec {
	if spec, ok := FacetIconSVGs[key]; ok {
		return spec
	}
	return FacetIconSVGs["fa-flag"]
}
