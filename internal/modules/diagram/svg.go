package diagram

import (
	"encoding/xml"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
)

// Colours are fixed (not themed) so a diagram reads the same in the studio, the
// wiki and a PDF. Styling is presentation attributes only: an inline <style>
// would leak into whatever page the SVG is embedded in.
const (
	fontFamily    = "Inter, 'Segoe UI', Helvetica, Arial, sans-serif"
	canvasBg      = "#ffffff"
	nodeStroke    = "#4b5563"
	nodeBody      = "#ffffff"
	headerSource  = "#34495e"
	headerTarget  = "#1f6f6b"
	headerText    = "#ffffff"
	portText      = "#1f2937"
	unmappedText  = "#6b7280"
	unsourcedText = "#b91c1c"
	ruleFill      = "#fff7e0"
	ruleStroke    = "#d4a017"
	ruleText      = "#6b4f00"
	edgeNeutral   = "#6b7280"
)

var edgePalette = []string{
	"#1f77b4", "#2ca02c", "#d62728", "#9467bd",
	"#ff7f0e", "#17becf", "#8c564b", "#e377c2",
}

// EdgeColour picks a palette colour from the label's first rule code (split on
// comma or whitespace, case-insensitive), so a code is the same colour in every
// diagram. An unlabelled edge is neutral grey.
func EdgeColour(label string) string {
	code := strings.FieldsFunc(strings.ToLower(label), func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' })
	if len(code) == 0 {
		return edgeNeutral
	}
	h := fnv.New32a()
	h.Write([]byte(code[0]))
	return edgePalette[h.Sum32()%uint32(len(edgePalette))]
}

func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// SVG renders a placed layout as a standalone SVG document; empty for an empty layout.
func SVG(l Layout) string {
	if len(l.Nodes) == 0 {
		return ""
	}
	labels := map[string]Node{}
	for _, n := range l.Nodes {
		labels[n.Node.ID] = n.Node
	}
	var b strings.Builder
	w, h := num(l.Width), num(l.Height)
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + w + `" height="` + h + `" viewBox="0 0 ` + w + ` ` + h + `" style="max-width:100%;height:auto" font-family="` + esc(fontFamily) + `" font-size="12">`)
	b.WriteString(`<rect width="` + w + `" height="` + h + `" fill="` + canvasBg + `"/>`)
	b.WriteString(`<g fill="none" stroke-width="1.4">`)
	for i, e := range l.Edges {
		b.WriteString(`<path data-edge="` + strconv.Itoa(i) + `" d="` + curve(e) + `" stroke="` + EdgeColour(e.Edge.Label) + `">`)
		b.WriteString(`<title>` + esc(endName(labels, e.Edge.From)+" -> "+endName(labels, e.Edge.To)))
		if e.Edge.Label != "" {
			b.WriteString(esc(" (" + e.Edge.Label + ")"))
		}
		b.WriteString(`</title></path>`)
	}
	b.WriteString(`</g>`)
	for _, n := range l.Nodes {
		if n.Node.Kind == KindRule {
			writeRule(&b, n)
		} else {
			writeEntity(&b, n)
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// curve chains horizontal-tangent cubics through the edge's lanes.
func curve(e EdgePath) string {
	pts := append([]Point{{e.X1, e.Y1}}, e.Via...)
	pts = append(pts, Point{e.X2, e.Y2})
	var b strings.Builder
	b.WriteString("M" + num(pts[0].X) + " " + num(pts[0].Y))
	for i := 1; i < len(pts); i++ {
		a, z := pts[i-1], pts[i]
		dx := max(20, (z.X-a.X)/2)
		b.WriteString(" C" + num(a.X+dx) + " " + num(a.Y) + "," + num(z.X-dx) + " " + num(z.Y) + "," + num(z.X) + " " + num(z.Y))
	}
	return b.String()
}

func endName(nodes map[string]Node, r PortRef) string {
	n := nodes[r.Node]
	name := DisplayLabel(n)
	if r.Port == "" {
		return name
	}
	for _, p := range n.Ports {
		if p.ID == r.Port {
			return name + "." + p.Label
		}
	}
	return name
}

func writeRule(b *strings.Builder, n NodeBox) {
	b.WriteString(`<g data-node="` + esc(n.Node.ID) + `">`)
	b.WriteString(`<rect x="` + num(n.X) + `" y="` + num(n.Y) + `" width="` + num(n.W) + `" height="` + num(n.H) +
		`" rx="` + num(n.H/2) + `" fill="` + ruleFill + `" stroke="` + ruleStroke + `"/>`)
	b.WriteString(`<text x="` + num(n.X+n.W/2) + `" y="` + num(n.Y+n.H/2+4) + `" text-anchor="middle" fill="` + ruleText + `">` + esc(DisplayLabel(n.Node)) + `</text>`)
	b.WriteString(`</g>`)
}

func writeEntity(b *strings.Builder, n NodeBox) {
	header := headerSource
	if n.Node.Layer == LayerTarget {
		header = headerTarget
	}
	b.WriteString(`<g data-node="` + esc(n.Node.ID) + `">`)
	b.WriteString(`<rect x="` + num(n.X) + `" y="` + num(n.Y) + `" width="` + num(n.W) + `" height="` + num(n.H) +
		`" rx="3" fill="` + nodeBody + `" stroke="` + nodeStroke + `"/>`)
	b.WriteString(`<path d="M` + num(n.X) + ` ` + num(n.Y+headerH) + ` V` + num(n.Y+3) + ` Q` + num(n.X) + ` ` + num(n.Y) + ` ` + num(n.X+3) + ` ` + num(n.Y) +
		` H` + num(n.X+n.W-3) + ` Q` + num(n.X+n.W) + ` ` + num(n.Y) + ` ` + num(n.X+n.W) + ` ` + num(n.Y+3) + ` V` + num(n.Y+headerH) + ` Z" fill="` + header + `"/>`)
	b.WriteString(`<text x="` + num(n.X+n.W/2) + `" y="` + num(n.Y+headerH/2+4) + `" text-anchor="middle" font-weight="bold" fill="` + headerText + `">` + esc(DisplayLabel(n.Node)) + `</text>`)
	for _, p := range n.Ports {
		fill, extra := portText, ""
		switch p.Port.Class {
		case PortUnmapped:
			fill, extra = unmappedText, ` font-style="italic"`
		case PortUnsourced:
			fill = unsourcedText
		}
		b.WriteString(`<g data-port="` + esc(p.Port.ID) + `"`)
		if p.Port.Class != "" {
			b.WriteString(` data-class="` + p.Port.Class + `"`)
		}
		b.WriteString(`>`)
		b.WriteString(`<text x="` + num(n.X+textPad) + `" y="` + num(p.Y+4) + `" fill="` + fill + `"` + extra + `>` + esc(p.Port.Label) + `</text>`)
		b.WriteString(`<rect x="` + num(n.X-3) + `" y="` + num(p.Y-3) + `" width="6" height="6" fill="` + nodeBody + `" stroke="` + nodeStroke + `"/>`)
		b.WriteString(`<rect x="` + num(n.X+n.W-3) + `" y="` + num(p.Y-3) + `" width="6" height="6" fill="` + nodeBody + `" stroke="` + nodeStroke + `"/>`)
		b.WriteString(`</g>`)
	}
	b.WriteString(`</g>`)
}
