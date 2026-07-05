package render

import (
	"encoding/xml"
	"html"
	"strings"
)

// maxSVGBytes caps an imported SVG so a giant file (e.g. one with embedded
// bitmaps) can't bloat a record or stall the sanitizer.
const maxSVGBytes = 512 * 1024

// svgAllowedElements is the default-deny allowlist of SVG elements (lowercased
// local names). Anything not here is dropped together with its whole subtree, so
// script/foreignObject/image and Inkscape's sodipodi:*/inkscape:* metadata never
// reach the output.
var svgAllowedElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true, "text": true, "tspan": true,
	"lineargradient": true, "radialgradient": true, "stop": true,
	"clippath": true, "mask": true, "use": true, "symbol": true,
	"marker": true, "pattern": true,
}

// svgAllowedAttrs is the allowlist of attributes (lowercased local names):
// geometry + presentation only. href/xlink:href, style and xmlns are handled
// specially in sanitizeSVGAttr; on* handlers and everything else are dropped.
var svgAllowedAttrs = map[string]bool{
	"d": true, "points": true, "x": true, "y": true, "x1": true, "y1": true,
	"x2": true, "y2": true, "cx": true, "cy": true, "r": true, "rx": true, "ry": true,
	"width": true, "height": true, "viewbox": true, "transform": true,
	"gradienttransform": true, "gradientunits": true, "offset": true,
	"fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true,
	"stroke-width": true, "stroke-linecap": true, "stroke-linejoin": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "stroke-opacity": true,
	"stroke-miterlimit": true, "opacity": true, "stop-color": true, "stop-opacity": true,
	"font-size": true, "font-family": true, "font-weight": true, "font-style": true,
	"text-anchor": true, "letter-spacing": true, "clip-path": true, "clip-rule": true,
	"mask": true, "id": true, "class": true, "preserveaspectratio": true, "version": true,
	"marker-start": true, "marker-mid": true, "marker-end": true, "display": true,
	"patternunits": true, "patterncontentunits": true, "maskunits": true,
	"markerwidth": true, "markerheight": true, "refx": true, "refy": true,
	"orient": true, "markerunits": true, "spreadmethod": true,
}

// svgSafeStyleProps are the CSS properties kept when sanitizing an inline
// style="" (Inkscape leans on these for fill/stroke); every other declaration is
// dropped so a hostile url()/expression/behavior can't ride in through style.
var svgSafeStyleProps = map[string]bool{
	"fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true,
	"stroke-width": true, "stroke-linecap": true, "stroke-linejoin": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "stroke-opacity": true,
	"stroke-miterlimit": true, "opacity": true, "stop-color": true, "stop-opacity": true,
	"font-size": true, "font-family": true, "font-weight": true, "font-style": true,
	"text-anchor": true, "display": true, "visibility": true, "color": true,
}

// sanitizeSVG turns untrusted SVG markup (e.g. an Inkscape export) into a safe
// subset: a default-deny walk that keeps only allowlisted elements/attributes,
// drops script/foreignObject/image/event-handlers, and allows references only to
// internal (#id) targets. Returns (clean, true) on success, ("", false) when the
// input is empty, too large, or has no <svg> root. The result is the security
// boundary; callers must not emit the raw input.
func sanitizeSVG(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxSVGBytes {
		return "", false
	}
	dec := xml.NewDecoder(strings.NewReader(raw))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	var out strings.Builder
	skipDepth := 0 // >0 means we are inside a dropped subtree
	seenSVG := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break // EOF or malformed: stop with what we have
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			name := strings.ToLower(t.Name.Local)
			if !svgAllowedElements[name] {
				skipDepth = 1
				continue
			}
			out.WriteByte('<')
			out.WriteString(name)
			if name == "svg" {
				seenSVG = true
				// A standalone SVG served as image/svg+xml needs xmlns declared or
				// an <img> won't decode it. Go's decoder folds the original xmlns
				// into the element namespace (it never reaches Attr), so emit it
				// once here; the xmlns attribute itself is dropped below.
				out.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
			}
			seenAttr := map[string]bool{}
			for _, a := range t.Attr {
				attr, key, ok := sanitizeSVGAttr(a)
				// Drop duplicates: Inkscape carries both version and
				// inkscape:version, which collapse to the same local name and
				// would otherwise emit a redefined attribute (invalid SVG).
				if !ok || seenAttr[key] {
					continue
				}
				seenAttr[key] = true
				out.WriteByte(' ')
				out.WriteString(attr)
			}
			out.WriteByte('>')
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			name := strings.ToLower(t.Name.Local)
			if svgAllowedElements[name] {
				out.WriteString("</")
				out.WriteString(name)
				out.WriteByte('>')
			}
		case xml.CharData:
			if skipDepth == 0 {
				out.WriteString(html.EscapeString(string(t)))
			}
			// comments, directives and processing instructions are ignored
		}
	}
	if !seenSVG {
		return "", false
	}
	return out.String(), true
}

const xlinkNS = "http://www.w3.org/1999/xlink"

// sanitizeSVGAttr returns (rendered, key, ok). key is the attribute's local name
// so the caller can drop duplicates; namespaced editor attributes, event
// handlers, xmlns and unsafe values are rejected (ok=false).
func sanitizeSVGAttr(a xml.Attr) (rendered, key string, ok bool) {
	// Drop namespaced editor attributes (inkscape:*, sodipodi:*, rdf:*). An
	// unprefixed SVG attribute has an empty Space; only xlink (for href) is
	// otherwise kept. Without this, inkscape:version collapses to the same local
	// name as version and emits a redefined attribute (invalid SVG).
	space := a.Name.Space
	if space != "" && !strings.EqualFold(space, xlinkNS) && !strings.EqualFold(space, "xlink") {
		return "", "", false
	}
	name := strings.ToLower(a.Name.Local)
	if strings.HasPrefix(name, "on") { // event handlers
		return "", "", false
	}
	if name == "xmlns" { // forced on the root <svg> in sanitizeSVG
		return "", "", false
	}
	switch name {
	case "href":
		v := strings.TrimSpace(a.Value)
		if !strings.HasPrefix(v, "#") { // internal refs only
			return "", "", false
		}
		return `href="` + html.EscapeString(v) + `"`, "href", true
	case "style":
		clean := sanitizeSVGStyle(a.Value)
		if clean == "" {
			return "", "", false
		}
		return `style="` + html.EscapeString(clean) + `"`, "style", true
	}
	if !svgAllowedAttrs[name] {
		return "", "", false
	}
	low := strings.ToLower(a.Value)
	if strings.Contains(low, "javascript:") || strings.Contains(low, "expression(") {
		return "", "", false
	}
	// A presentation value may legitimately be url(#id) (a gradient/clip ref);
	// any external url() is rejected.
	if strings.Contains(low, "url(") && !urlRefsInternalOnly(low) {
		return "", "", false
	}
	return name + `="` + html.EscapeString(a.Value) + `"`, name, true
}

// urlRefsInternalOnly reports whether every url(...) in a value points at an
// internal fragment (#id). External url() is a fetch vector and rejected.
func urlRefsInternalOnly(low string) bool {
	rest := low
	for {
		i := strings.Index(rest, "url(")
		if i < 0 {
			return true
		}
		rest = rest[i+4:]
		rest = strings.TrimLeft(rest, " '\"")
		if !strings.HasPrefix(rest, "#") {
			return false
		}
	}
}

// sanitizeSVGStyle keeps only safe declarations from an inline style value.
func sanitizeSVGStyle(style string) string {
	var kept []string
	for _, decl := range strings.Split(style, ";") {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}
		colon := strings.Index(decl, ":")
		if colon < 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(decl[:colon]))
		val := strings.TrimSpace(decl[colon+1:])
		low := strings.ToLower(val)
		if !svgSafeStyleProps[prop] {
			continue
		}
		if strings.Contains(low, "javascript:") || strings.Contains(low, "expression(") || strings.Contains(low, "@import") {
			continue
		}
		if strings.Contains(low, "url(") && !urlRefsInternalOnly(low) {
			continue
		}
		kept = append(kept, prop+":"+val)
	}
	return strings.Join(kept, ";")
}
