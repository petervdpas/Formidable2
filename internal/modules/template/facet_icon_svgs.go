package template

import (
	"embed"
	"fmt"
	iofs "io/fs"
	"regexp"
	"strings"
)

// FacetIconSpec carries the minimal data needed to render one
// FontAwesome glyph as inline SVG: the path's `viewBox` (icons have
// different aspects) and the geometry in `Path`'s `d` attribute. No
// `xmlns` and no fill — both layers (wiki + Vue) wrap this in their
// own colour-aware shell.
type FacetIconSpec struct {
	ViewBox string `json:"viewBox"`
	Path    string `json:"path"`
}

// FacetIconSVGs is the parsed catalog of every key in FacetIconList,
// keyed by the "fa-" prefixed form ("fa-flag", "fa-shirt", …) so it
// can drop into HTML / JS without further mapping. The map is computed
// once at init() from `icons/*.svg`; missing files panic at startup
// so a missing glyph never lands in production silently.
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

// parseFacetIconSVG pulls the viewBox and the first <path d="…"/> out
// of an SVG byte slice. The upstream FontAwesome SVGs are
// well-formed and contain exactly one path with a `d` attribute,
// which is the only data we care about here. Anything more elaborate
// — gradients, multiple paths — would need a real parser, but the
// closed icon palette never uses those.
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

// FacetIconSpecFor returns the parsed spec for a given key, falling
// back to fa-flag for unknown / empty input so a stale template
// reference still renders a real glyph rather than a void.
func FacetIconSpecFor(key string) FacetIconSpec {
	if spec, ok := FacetIconSVGs[key]; ok {
		return spec
	}
	return FacetIconSVGs["fa-flag"]
}
