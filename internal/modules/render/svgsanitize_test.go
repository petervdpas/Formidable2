package render

import (
	"strings"
	"testing"
)

func TestSanitizeSVG_StripsActiveContent(t *testing.T) {
	raw := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">
		<script>alert(1)</script>
		<foreignObject><body onload="x()">hi</body></foreignObject>
		<image href="http://evil/x.png" x="0" y="0" width="10" height="10"/>
		<rect x="0" y="0" width="10" height="10" fill="red" onload="steal()" onclick="go()"/>
		<a href="javascript:alert(1)"><text>click</text></a>
	</svg>`
	got, ok := sanitizeSVG(raw)
	if !ok {
		t.Fatalf("expected sanitize to succeed")
	}
	for _, bad := range []string{"script", "alert", "foreignObject", "onload", "onclick", "<image", "<body", "javascript:", "evil"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitized SVG still contains %q\n%s", bad, got)
		}
	}
	// the safe geometry survives
	if !strings.Contains(got, "<rect") || !strings.Contains(got, `fill="red"`) {
		t.Errorf("safe rect/fill should survive\n%s", got)
	}
}

func TestSanitizeSVG_ReferencesAndStyle(t *testing.T) {
	raw := `<svg xmlns="http://www.w3.org/2000/svg">
		<defs><linearGradient id="g"><stop offset="0" stop-color="#f00"/></linearGradient></defs>
		<use href="#g"/>
		<use href="http://evil/x"/>
		<rect width="10" height="10" fill="url(#g)" style="fill:#0f0;background:url(http://evil);opacity:.5"/>
		<circle cx="5" cy="5" r="4" fill="url(http://evil/x)"/>
	</svg>`
	got, ok := sanitizeSVG(raw)
	if !ok {
		t.Fatalf("expected sanitize to succeed")
	}
	if !strings.Contains(got, `href="#g"`) {
		t.Errorf("internal #ref should survive\n%s", got)
	}
	if strings.Contains(got, "evil") {
		t.Errorf("external refs (href/url) must be dropped\n%s", got)
	}
	if !strings.Contains(got, `fill="url(#g)"`) {
		t.Errorf("internal url(#id) paint should survive\n%s", got)
	}
	// style: safe decl kept, external-url decl dropped
	if !strings.Contains(got, "fill:#0f0") {
		t.Errorf("safe style declaration should survive\n%s", got)
	}
	if strings.Contains(got, "background") {
		t.Errorf("unsafe style declaration should be dropped\n%s", got)
	}
	// circle with external url() fill: the whole attr is dropped, not the element
	if !strings.Contains(got, "<circle") {
		t.Errorf("circle element should survive even when its fill is rejected\n%s", got)
	}
}

func TestSanitizeSVG_DropsInkscapeMetadata(t *testing.T) {
	raw := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:sodipodi="x" xmlns:inkscape="y">
		<sodipodi:namedview inkscape:zoom="4"/>
		<metadata><rdf/></metadata>
		<path d="M0 0 L10 10" inkscape:label="layer1" fill="black"/>
	</svg>`
	got, ok := sanitizeSVG(raw)
	if !ok {
		t.Fatalf("expected sanitize to succeed")
	}
	for _, bad := range []string{"namedview", "sodipodi", "inkscape", "metadata", "rdf"} {
		if strings.Contains(got, bad) {
			t.Errorf("editor metadata %q should be dropped\n%s", bad, got)
		}
	}
	if !strings.Contains(got, `<path d="M0 0 L10 10"`) || !strings.Contains(got, `fill="black"`) {
		t.Errorf("the actual path should survive\n%s", got)
	}
}

func TestSanitizeSVG_ValidStandaloneOutput(t *testing.T) {
	// An Inkscape root carries version + inkscape:version (same local name) and
	// its xmlns folds into the namespace. The output must declare xmlns exactly
	// once and never emit a duplicate attribute (both break an <img> load).
	raw := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" version="1.1" inkscape:version="1.0" viewBox="0 0 10 10"><rect width="10" height="10" fill="red"/></svg>`
	got, ok := sanitizeSVG(raw)
	if !ok {
		t.Fatalf("expected sanitize to succeed")
	}
	if n := strings.Count(got, "xmlns="); n != 1 {
		t.Errorf("xmlns must appear exactly once, got %d\n%s", n, got)
	}
	if !strings.Contains(got, `xmlns="http://www.w3.org/2000/svg"`) {
		t.Errorf("standalone SVG must declare the svg namespace\n%s", got)
	}
	if n := strings.Count(got, "version="); n != 1 {
		t.Errorf("version must not be redefined, got %d occurrences\n%s", n, got)
	}
	if strings.Contains(got, "inkscape") {
		t.Errorf("inkscape:version should be dropped\n%s", got)
	}
}

func TestSanitizeSVG_RejectsJunk(t *testing.T) {
	if _, ok := sanitizeSVG(""); ok {
		t.Errorf("empty input should fail")
	}
	if _, ok := sanitizeSVG("<div>not svg</div>"); ok {
		t.Errorf("non-svg input should fail")
	}
	if _, ok := sanitizeSVG("<svg>" + strings.Repeat("a", maxSVGBytes) + "</svg>"); ok {
		t.Errorf("oversized input should be rejected")
	}
}
