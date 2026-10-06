package diagram

import (
	"encoding/xml"
	"hash/fnv"
	"math"
	"sort"
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
	ruleFill      = "#ffffff"
	literalFill   = "#f3f4f6"
	literalStroke = "#6b7280"
	literalText   = "#1f2937"
	edgeNeutral   = "#6b7280"
	legendH       = 28.0
)

// categoryColours pins the transaction-rule categories (Compound, Enumerate,
// Filter, Mapping, Query, Validation) to distinct colours; any other category
// hashes into edgePalette.
var categoryColours = map[string]string{
	"C": "#d62728", "E": "#9467bd", "F": "#ff7f0e",
	"M": "#2ca02c", "Q": "#1f77b4", "V": "#8c564b",
}

var edgePalette = []string{
	"#17becf", "#e377c2", "#bcbd22", "#7f7f7f",
	"#393b79", "#637939", "#843c39", "#7b4173",
}

// category is a rule code's leading letters, upper-cased ("q-01" -> "Q").
func category(code string) string {
	code = strings.TrimSpace(code)
	i := 0
	for i < len(code) && (code[i] >= 'a' && code[i] <= 'z' || code[i] >= 'A' && code[i] <= 'Z') {
		i++
	}
	return strings.ToUpper(code[:i])
}

func categoryColour(cat string) string {
	if cat == "" {
		return edgeNeutral
	}
	if c, ok := categoryColours[cat]; ok {
		return c
	}
	h := fnv.New32a()
	h.Write([]byte(cat))
	return edgePalette[h.Sum32()%uint32(len(edgePalette))]
}

// EdgeColour is the category colour of the label's first listed rule code;
// neutral grey without one.
func EdgeColour(label string) string {
	codes := splitCodes(label)
	if len(codes) == 0 {
		return edgeNeutral
	}
	return categoryColour(category(codes[0]))
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
	cats := legendCategories(l)
	height := l.Height
	if len(cats) > 0 {
		height += legendH
	}
	var b strings.Builder
	w, h := num(l.Width), num(height)
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + w + `" height="` + h + `" viewBox="0 0 ` + w + ` ` + h + `" style="max-width:100%;height:auto" font-family="` + esc(fontFamily) + `" font-size="12">`)
	b.WriteString(`<rect width="` + w + `" height="` + h + `" fill="` + canvasBg + `"/>`)
	b.WriteString(`<g fill="none" stroke-width="1.4">`)
	for i, e := range l.Edges {
		b.WriteString(`<path data-edge="` + strconv.Itoa(i) + `" d="` + curve(e) + `" stroke="` + EdgeColour(e.Edge.Label) + `">`)
		b.WriteString(`<title>` + esc(endName(labels, e.Edge.From)+" -> "+endName(labels, e.Edge.To)))
		if e.Edge.Label != "" {
			b.WriteString(esc(" (" + e.Edge.Label + ")"))
		}
		if e.Edge.Note != "" {
			b.WriteString(esc(" = " + e.Edge.Note))
		}
		b.WriteString(`</title></path>`)
	}
	b.WriteString(`</g>`)
	for _, n := range l.Nodes {
		switch n.Node.Kind {
		case KindRule:
			c := categoryColour(category(n.Node.Label))
			if n.Node.Class == NodeUndefined {
				writePill(&b, n, ruleFill, unsourcedText, unsourcedText, ` stroke-dasharray="4 3"`)
			} else {
				writePill(&b, n, ruleFill, c, c, "")
			}
		case KindLiteral:
			writePill(&b, n, literalFill, literalStroke, literalText, ` stroke-dasharray="4 3"`)
		default:
			writeEntity(&b, n)
		}
	}
	writeLegend(&b, cats, l.Height)
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

// writePill draws a rounded node; a fragment Link ("#id") wraps it in an
// in-page anchor, any other link is ignored.
func writePill(b *strings.Builder, n NodeBox, fill, stroke, text, extra string) {
	link := strings.HasPrefix(n.Node.Link, "#") && len(n.Node.Link) > 1
	if link {
		b.WriteString(`<a href="` + esc(n.Node.Link) + `">`)
	}
	b.WriteString(`<g data-node="` + esc(n.Node.ID) + `"`)
	if n.Node.Class != "" {
		b.WriteString(` data-class="` + esc(n.Node.Class) + `"`)
	}
	b.WriteString(`>`)
	b.WriteString(`<rect x="` + num(n.X) + `" y="` + num(n.Y) + `" width="` + num(n.W) + `" height="` + num(n.H) +
		`" rx="` + num(n.H/2) + `" fill="` + fill + `" stroke="` + stroke + `" stroke-width="1.4"` + extra + `/>`)
	b.WriteString(`<text x="` + num(n.X+n.W/2) + `" y="` + num(n.Y+n.H/2+4) + `" text-anchor="middle" fill="` + text + `">` + esc(DisplayLabel(n.Node)) + `</text>`)
	b.WriteString(`</g>`)
	if link {
		b.WriteString(`</a>`)
	}
}

// legendCategories lists the rule categories used by edges, sorted.
func legendCategories(l Layout) []string {
	seen := map[string]bool{}
	for _, e := range l.Edges {
		for _, code := range splitCodes(e.Edge.Label) {
			if c := category(code); c != "" {
				seen[c] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// writeLegend draws one swatch per category in a strip below the diagram.
func writeLegend(b *strings.Builder, cats []string, top float64) {
	x, y := margin, top+legendH/2
	for _, c := range cats {
		col := categoryColour(c)
		b.WriteString(`<g data-legend="` + esc(c) + `">`)
		b.WriteString(`<line x1="` + num(x) + `" y1="` + num(y) + `" x2="` + num(x+22) + `" y2="` + num(y) + `" stroke="` + col + `" stroke-width="3"/>`)
		b.WriteString(`<text x="` + num(x+28) + `" y="` + num(y+4) + `" fill="` + portText + `">` + esc(c) + `</text>`)
		b.WriteString(`</g>`)
		x += 28 + textW(c) + 24
	}
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
