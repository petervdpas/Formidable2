package template

import (
	"strings"
	"testing"
)

// Every key in FacetIconList must resolve to a non-empty parsed spec.
// init() panics on any missing file, so this test would only ever
// fail if FacetIconSVGs were modified post-init - but it keeps the
// closed-set contract visible alongside the other facet validation
// tests.
func TestFacetIconSVGs_CoverEveryListedKey(t *testing.T) {
	for _, key := range FacetIconList {
		spec, ok := FacetIconSVGs[key]
		if !ok {
			t.Errorf("FacetIconList key %q missing from FacetIconSVGs", key)
			continue
		}
		if spec.ViewBox == "" || spec.Path == "" {
			t.Errorf("spec for %q is incomplete: viewBox=%q pathBytes=%d",
				key, spec.ViewBox, len(spec.Path))
		}
	}
}

// FacetIconSpecFor must round-trip every known key and fall back to
// fa-flag for unknown / empty input rather than returning a zero
// value (which would render an invisible <svg/>).
func TestFacetIconSpecFor_KnownAndFallback(t *testing.T) {
	flag := FacetIconSVGs["fa-flag"]
	for _, key := range FacetIconList {
		if got := FacetIconSpecFor(key); got != FacetIconSVGs[key] {
			t.Errorf("FacetIconSpecFor(%q) returned %#v, want %#v", key, got, FacetIconSVGs[key])
		}
	}
	for _, bad := range []string{"", "fa-unknown", "nonsense"} {
		if got := FacetIconSpecFor(bad); got != flag {
			t.Errorf("FacetIconSpecFor(%q) did not fall back to fa-flag", bad)
		}
	}
}

// Sanity: a representative parse extracts both the viewBox and the
// path data correctly. fa-shirt has a 0 0 640 512 viewBox - picked
// because it's the only icon with a non-default width.
func TestFacetIconSVGs_ShirtParsedCorrectly(t *testing.T) {
	spec := FacetIconSpecFor("fa-shirt")
	if spec.ViewBox != "0 0 640 512" {
		t.Errorf("fa-shirt viewBox = %q, want %q", spec.ViewBox, "0 0 640 512")
	}
	if !strings.HasPrefix(spec.Path, "M320.2 112c") {
		t.Errorf("fa-shirt path didn't start with the expected prefix; got %q", spec.Path[:32])
	}
}

// Parser must reject SVGs that don't have a viewBox + first <path d="…"/>
// - guards against a future maintainer dropping a more complex multi-
// path or gradient SVG into icons/ without realising it.
func TestParseFacetIconSVG_RejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"no viewBox", `<svg><path d="M1 1 z"/></svg>`},
		{"no path", `<svg viewBox="0 0 1 1"></svg>`},
		{"empty", ``},
	}
	for _, tc := range cases {
		_, err := parseFacetIconSVG([]byte(tc.body))
		if err == nil {
			t.Errorf("%s: expected parse error, got nil", tc.name)
		}
	}
}
