package render

import (
	"strings"
	"testing"
)

func TestSanitizeSVG_PassesToolOutputVerbatim(t *testing.T) {
	// A realistic export (folded xmlns, inkscape attrs, an Illustrator-style
	// <style> block with classes, a gradient) must survive byte-for-byte: no
	// rewrite, so no tool's markup is mangled.
	raw := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" version="1.1" inkscape:version="1.0" width="10" height="10" viewBox="0 0 10 10"><style>.a{fill:#f00}</style><defs><linearGradient id="g"><stop offset="0" stop-color="#00f"/></linearGradient></defs><rect class="a" width="10" height="10"/><circle cx="5" cy="5" r="3" fill="url(#g)"/></svg>`
	got, ok := sanitizeSVG(raw)
	if !ok {
		t.Fatalf("a clean tool SVG must be accepted")
	}
	if got != raw {
		t.Errorf("a sized SVG must be stored verbatim\nwant %q\ngot  %q", raw, got)
	}
}

func TestSanitizeSVG_AddsIntrinsicSizeFromViewBox(t *testing.T) {
	// A viewBox-only SVG gets width/height from the viewBox so it has an intrinsic
	// size (an <img> probe reports zero otherwise); everything else is untouched.
	got, ok := sanitizeSVG(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 120"><rect width="200" height="120"/></svg>`)
	if !ok {
		t.Fatalf("expected accept")
	}
	if !strings.Contains(got, `width="200"`) || !strings.Contains(got, `height="120"`) {
		t.Errorf("width/height should be derived from the viewBox\n%s", got)
	}
	if !strings.Contains(got, `viewBox="0 0 200 120"`) || !strings.Contains(got, "<rect") {
		t.Errorf("the rest of the SVG must be preserved\n%s", got)
	}

	// A sized SVG is not touched (no duplicate dimensions).
	sized := `<svg xmlns="http://www.w3.org/2000/svg" width="50" height="40" viewBox="0 0 200 120"><rect/></svg>`
	if out, _ := sanitizeSVG(sized); out != sized {
		t.Errorf("a sized SVG must be left as is\nwant %q\ngot  %q", sized, out)
	}

	// A stroke-width attribute must not be mistaken for a width dimension.
	vbOnly := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 4"><path d="M0 0" stroke-width="2"/></svg>`
	if out, _ := sanitizeSVG(vbOnly); !strings.Contains(out, `<svg width="8" height="4"`) {
		t.Errorf("stroke-width must not block size derivation\n%s", out)
	}
}

func TestSanitizeSVG_RejectsActiveContent(t *testing.T) {
	cases := map[string]string{
		"script":        `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"foreignObject": `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><b/></foreignObject></svg>`,
		"handler":       `<svg xmlns="http://www.w3.org/2000/svg"><rect onload="x()"/></svg>`,
		"js-href":       `<svg xmlns="http://www.w3.org/2000/svg"><a href="javascript:alert(1)"><rect/></a></svg>`,
	}
	for name, raw := range cases {
		if _, ok := sanitizeSVG(raw); ok {
			t.Errorf("%s: active content must be rejected", name)
		}
	}
}

func TestSanitizeSVG_RejectsActiveContentEvenWhenUnparseable(t *testing.T) {
	// A DOCTYPE + entity that could trip the strict reader must still be scanned,
	// so a script can't hide behind a parse error.
	raw := `<!DOCTYPE svg [<!ENTITY x "y">]><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
	if _, ok := sanitizeSVG(raw); ok {
		t.Errorf("script must be caught even when the document is awkward to parse")
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
