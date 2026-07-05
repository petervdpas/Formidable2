package render

import (
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

// maxSVGBytes caps an imported SVG so a giant file (e.g. one with embedded
// bitmaps) can't bloat a record or stall the check.
const maxSVGBytes = 512 * 1024

// onHandlerAttr matches an inline event-handler attribute (onload=, onclick=…),
// used only as a fallback when the SVG can't be token-parsed.
var onHandlerAttr = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)

// sanitizeSVG accepts SVG markup from any tool (Inkscape, Illustrator, Boxy, …)
// and returns it VERBATIM when safe to store, or ("", false) when it is not an
// SVG, is too large, or carries active content (script / foreignObject / event
// handlers / javascript: URLs).
//
// It deliberately does NOT rewrite the markup. Re-serializing tool-specific
// output (folded namespaces, <style> blocks, filter stacks, class-based styling)
// is what mangles real files, and an allowlist can never track every tool.
// Safety comes from how the file is used, not from neutering it: imported SVGs
// render through <img>, which browsers run in secure static mode (no scripts, no
// external fetches), and the image route serves them under a restrictive CSP so
// even direct navigation can't execute script. This gate just refuses an
// obviously-active file rather than trying to clean it in place.
func sanitizeSVG(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxSVGBytes {
		return "", false
	}
	if !strings.Contains(strings.ToLower(raw), "<svg") {
		return "", false
	}
	if svgHasActiveContent(raw) {
		return "", false
	}
	return ensureSVGIntrinsicSize(raw), true
}

var svgOpenTagRe = regexp.MustCompile(`(?is)<svg\b[^>]*>`)
var svgWidthAttrRe = regexp.MustCompile(`(?i)\swidth\s*=`)   // leading space avoids matching stroke-width=
var svgHeightAttrRe = regexp.MustCompile(`(?i)\sheight\s*=`) // (no other -height attr in SVG)
var svgViewBoxRe = regexp.MustCompile(`(?i)\sviewbox\s*=\s*["']([^"']*)["']`)

// ensureSVGIntrinsicSize adds width/height to the root <svg> when it has a
// viewBox but neither dimension, so the file has an intrinsic size for an <img>
// probe (some engines report a zero natural size otherwise). It is additive and
// touches only the opening tag; a sized or viewBox-less SVG is returned as is,
// so the sanitizer stays otherwise verbatim.
func ensureSVGIntrinsicSize(svg string) string {
	loc := svgOpenTagRe.FindStringIndex(svg)
	if loc == nil {
		return svg
	}
	tag := svg[loc[0]:loc[1]]
	if svgWidthAttrRe.MatchString(tag) || svgHeightAttrRe.MatchString(tag) {
		return svg // already carries a dimension; leave it alone
	}
	m := svgViewBoxRe.FindStringSubmatch(tag)
	if m == nil {
		return svg // nothing to derive a size from
	}
	nums := strings.FieldsFunc(m[1], func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(nums) != 4 {
		return svg
	}
	newTag := "<svg width=\"" + nums[2] + "\" height=\"" + nums[3] + "\"" + tag[len("<svg"):]
	return svg[:loc[0]] + newTag + svg[loc[1]:]
}

// svgHasActiveContent reports whether the markup contains a script-execution
// vector. It walks the XML for accuracy and falls back to a lexical scan when the
// document doesn't parse (some tools emit a DOCTYPE or custom entities the strict
// reader trips on), so a malicious file can't hide behind a parse error.
func svgHasActiveContent(raw string) bool {
	dec := xml.NewDecoder(strings.NewReader(raw))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return false
		}
		if err != nil {
			return svgHasActiveContentLexical(raw)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch strings.ToLower(se.Name.Local) {
		case "script", "foreignobject":
			return true
		}
		for _, a := range se.Attr {
			if strings.HasPrefix(strings.ToLower(a.Name.Local), "on") {
				return true
			}
			val := strings.ToLower(a.Value)
			if strings.Contains(val, "javascript:") || strings.Contains(val, "expression(") {
				return true
			}
		}
	}
}

func svgHasActiveContentLexical(raw string) bool {
	low := strings.ToLower(raw)
	if strings.Contains(low, "<script") || strings.Contains(low, "<foreignobject") ||
		strings.Contains(low, "javascript:") || strings.Contains(low, "expression(") {
		return true
	}
	return onHandlerAttr.MatchString(raw)
}
