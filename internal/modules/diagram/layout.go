package diagram

import (
	"sort"
	"strconv"
	"unicode/utf8"
)

// Layout metrics, in SVG user units. Text width is estimated (no font metrics
// in Go), so charW leans generous to keep labels inside their boxes.
const (
	margin    = 24.0
	colGap    = 200.0
	nodeGap   = 28.0
	headerH   = 26.0
	portH     = 20.0
	portPadB  = 6.0
	charW     = 7.0
	textPad   = 14.0
	nodeMinW  = 120.0
	ruleH     = 26.0
	ruleMinW  = 60.0
	dummyH    = 4.0
	dummyGap  = 8.0
	sweeps    = 12
	unnamedID = "(?)"
	kindDummy = "~dummy"
)

type Layout struct {
	Width  float64    `json:"width"`
	Height float64    `json:"height"`
	Nodes  []NodeBox  `json:"nodes"`
	Edges  []EdgePath `json:"edges"`
}

// NodeBox is a placed node; Ports are in display order with absolute centre Y.
type NodeBox struct {
	Node  Node      `json:"node"`
	X     float64   `json:"x"`
	Y     float64   `json:"y"`
	W     float64   `json:"w"`
	H     float64   `json:"h"`
	Ports []PortBox `json:"ports"`
}

type PortBox struct {
	Port Port    `json:"port"`
	Y    float64 `json:"y"`
}

// EdgePath runs from (X1,Y1) through Via to (X2,Y2). Via holds the lane an
// edge takes through each column it skips, so it never runs behind a node.
type EdgePath struct {
	Edge Edge    `json:"edge"`
	X1   float64 `json:"x1"`
	Y1   float64 `json:"y1"`
	X2   float64 `json:"x2"`
	Y2   float64 `json:"y2"`
	Via  []Point `json:"via,omitempty"`
	cols []int
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// DisplayLabel is what a node header shows; an unnamed node reads "(?)".
func DisplayLabel(n Node) string {
	if n.Label == "" {
		return unnamedID
	}
	return n.Label
}

// Place lays the graph out in columns (one per layer, unknown layers appended in
// first-seen order) and orders nodes and their ports by barycenter sweeps,
// keeping the ordering with the fewest crossings. Pure and deterministic.
func Place(g Graph) Layout {
	p := newPlacer(g)
	if len(p.boxes) == 0 {
		return Layout{Nodes: []NodeBox{}, Edges: []EdgePath{}}
	}
	p.stack()
	best, bestX := p.snapshot(), p.crossings()
	for i := 0; i < sweeps && bestX > 0; i++ {
		p.sweep(i%2 == 0)
		if x := p.crossings(); x < bestX {
			best, bestX = p.snapshot(), x
		}
	}
	p.restore(best)
	return p.layout()
}

// Crossings counts pairs of edge segments that cross between the same pair of columns.
func Crossings(l Layout) int {
	type seg struct {
		c1, c2 int
		y1, y2 float64
	}
	var segs []seg
	for _, e := range l.Edges {
		ys := make([]float64, 0, len(e.Via)+2)
		ys = append(ys, e.Y1)
		for _, v := range e.Via {
			ys = append(ys, v.Y)
		}
		ys = append(ys, e.Y2)
		for i := 0; i+1 < len(ys) && i+1 < len(e.cols); i++ {
			segs = append(segs, seg{e.cols[i], e.cols[i+1], ys[i], ys[i+1]})
		}
	}
	n := 0
	for i := range segs {
		for j := i + 1; j < len(segs); j++ {
			a, b := segs[i], segs[j]
			if a.c1 == b.c1 && a.c2 == b.c2 && (a.y1-b.y1)*(a.y2-b.y2) < 0 {
				n++
			}
		}
	}
	return n
}

// placer works on segments between adjacent columns: an edge that skips
// columns is split through one dummy box per skipped column (its chain).
type placer struct {
	cols   [][]*NodeBox
	colOf  map[string]int
	boxes  map[string]*NodeBox
	order  []string
	edges  []Edge
	chains [][]PortRef
	segs   []Edge
}

func newPlacer(g Graph) *placer {
	p := &placer{boxes: map[string]*NodeBox{}, colOf: map[string]int{}}
	colIdx := map[string]int{}
	for _, l := range g.Layers {
		if _, ok := colIdx[l.ID]; !ok {
			colIdx[l.ID] = len(p.cols)
			p.cols = append(p.cols, nil)
		}
	}
	for _, n := range g.Nodes {
		if _, dup := p.boxes[n.ID]; dup {
			continue
		}
		ci, ok := colIdx[n.Layer]
		if !ok {
			ci = len(p.cols)
			colIdx[n.Layer] = ci
			p.cols = append(p.cols, nil)
		}
		b := &NodeBox{Node: n, Ports: make([]PortBox, len(n.Ports))}
		for i, pt := range n.Ports {
			b.Ports[i] = PortBox{Port: pt}
		}
		size(b)
		p.boxes[n.ID] = b
		p.order = append(p.order, n.ID)
		p.cols[ci] = append(p.cols[ci], b)
		p.colOf[n.ID] = ci
	}
	for _, e := range g.Edges {
		if p.anchorOK(e.From) && p.anchorOK(e.To) {
			p.addEdge(e)
		}
	}
	x := margin
	for _, col := range p.cols {
		w := 0.0
		for _, b := range col {
			if b.W > w {
				w = b.W
			}
		}
		for _, b := range col {
			b.X = x + (w-b.W)/2
		}
		if len(col) > 0 {
			x += w + colGap
		}
	}
	return p
}

func (p *placer) addEdge(e Edge) {
	chain := []PortRef{e.From}
	from, to := p.colOf[e.From.Node], p.colOf[e.To.Node]
	for c := from + 1; c < to; c++ {
		id := kindDummy + strconv.Itoa(len(p.boxes))
		d := &NodeBox{Node: Node{ID: id, Kind: kindDummy}, H: dummyH, Ports: []PortBox{}}
		p.boxes[id] = d
		p.cols[c] = append(p.cols[c], d)
		p.colOf[id] = c
		chain = append(chain, PortRef{Node: id})
	}
	chain = append(chain, e.To)
	for i := 0; i+1 < len(chain); i++ {
		p.segs = append(p.segs, Edge{From: chain[i], To: chain[i+1]})
	}
	p.edges = append(p.edges, e)
	p.chains = append(p.chains, chain)
}

func (p *placer) anchorOK(r PortRef) bool {
	b, ok := p.boxes[r.Node]
	if !ok {
		return false
	}
	if r.Port == "" {
		return true
	}
	for _, pb := range b.Ports {
		if pb.Port.ID == r.Port {
			return true
		}
	}
	return false
}

func textW(s string) float64 { return float64(utf8.RuneCountInString(s)) * charW }

func size(b *NodeBox) {
	if b.Node.Kind == KindRule {
		b.W = max(ruleMinW, textW(DisplayLabel(b.Node))+2*textPad)
		b.H = ruleH
		return
	}
	w := textW(DisplayLabel(b.Node)) * 1.1
	for _, pb := range b.Ports {
		w = max(w, textW(pb.Port.Label))
	}
	b.W = max(nodeMinW, w+2*textPad)
	b.H = headerH + float64(len(b.Ports))*portH + portPadB
}

// stack assigns Y top-down per column, then centres each column on the tallest.
func (p *placer) stack() {
	heights := make([]float64, len(p.cols))
	tallest := 0.0
	for ci, col := range p.cols {
		h := 0.0
		for i, b := range col {
			if i > 0 {
				h += gap(col[i-1], b)
			}
			h += b.H
		}
		heights[ci] = h
		tallest = max(tallest, h)
	}
	for ci, col := range p.cols {
		y := margin + (tallest-heights[ci])/2
		for i, b := range col {
			if i > 0 {
				y += gap(col[i-1], b)
			}
			b.Y = y
			for i := range b.Ports {
				b.Ports[i].Y = y + headerH + float64(i)*portH + portH/2
			}
			y += b.H
		}
	}
}

func gap(a, b *NodeBox) float64 {
	if a.Node.Kind == kindDummy && b.Node.Kind == kindDummy {
		return dummyGap
	}
	return nodeGap
}

func (p *placer) anchor(r PortRef) (x, y float64, b *NodeBox) {
	b = p.boxes[r.Node]
	if r.Port != "" {
		for _, pb := range b.Ports {
			if pb.Port.ID == r.Port {
				return b.X, pb.Y, b
			}
		}
	}
	if b.Node.Kind != KindEntity || len(b.Ports) == 0 {
		return b.X, b.Y + b.H/2, b
	}
	return b.X, b.Y + headerH/2, b
}

// sweep reorders every column by the barycenter of its neighbours on one side:
// forward looks left (inbound edges), backward looks right (outbound).
func (p *placer) sweep(forward bool) {
	idx := make([]int, len(p.cols))
	for i := range idx {
		idx[i] = i
	}
	if !forward {
		for i, j := 0, len(idx)-1; i < j; i, j = i+1, j-1 {
			idx[i], idx[j] = idx[j], idx[i]
		}
	}
	for _, ci := range idx {
		p.reorderColumn(ci, forward)
		p.stack()
	}
}

func (p *placer) reorderColumn(ci int, forward bool) {
	col := p.cols[ci]
	inCol := map[string]bool{}
	for _, b := range col {
		inCol[b.Node.ID] = true
	}
	portSum := map[PortRef][2]float64{}
	nodeSum := map[string][2]float64{}
	for _, e := range p.segs {
		self, other := e.To, e.From
		if !forward {
			self, other = e.From, e.To
		}
		if !inCol[self.Node] || inCol[other.Node] {
			continue
		}
		_, oy, _ := p.anchor(other)
		if self.Port != "" {
			s := portSum[self]
			portSum[self] = [2]float64{s[0] + oy, s[1] + 1}
		}
		s := nodeSum[self.Node]
		nodeSum[self.Node] = [2]float64{s[0] + oy, s[1] + 1}
	}
	keys := map[string]float64{}
	for _, b := range col {
		pk := make(map[string]float64, len(b.Ports))
		for _, pb := range b.Ports {
			pk[pb.Port.ID] = pb.Y
			if s := portSum[PortRef{Node: b.Node.ID, Port: pb.Port.ID}]; s[1] > 0 {
				pk[pb.Port.ID] = s[0] / s[1]
			}
		}
		sort.SliceStable(b.Ports, func(i, j int) bool { return pk[b.Ports[i].Port.ID] < pk[b.Ports[j].Port.ID] })
		keys[b.Node.ID] = b.Y + b.H/2
		if s := nodeSum[b.Node.ID]; s[1] > 0 {
			keys[b.Node.ID] = s[0] / s[1]
		}
	}
	sort.SliceStable(col, func(i, j int) bool { return keys[col[i].Node.ID] < keys[col[j].Node.ID] })
}

type snap struct {
	cols  [][]*NodeBox
	ports map[string][]PortBox
}

func (p *placer) snapshot() snap {
	s := snap{cols: make([][]*NodeBox, len(p.cols)), ports: map[string][]PortBox{}}
	for i, col := range p.cols {
		s.cols[i] = append([]*NodeBox(nil), col...)
	}
	for id, b := range p.boxes {
		s.ports[id] = append([]PortBox(nil), b.Ports...)
	}
	return s
}

func (p *placer) restore(s snap) {
	for i := range p.cols {
		p.cols[i] = append([]*NodeBox(nil), s.cols[i]...)
	}
	for id, b := range p.boxes {
		b.Ports = append([]PortBox(nil), s.ports[id]...)
	}
	p.stack()
}

func (p *placer) paths() []EdgePath {
	out := make([]EdgePath, 0, len(p.edges))
	for i, e := range p.edges {
		chain := p.chains[i]
		x1, y1, fb := p.anchor(e.From)
		x2, y2, _ := p.anchor(e.To)
		ep := EdgePath{Edge: e, X1: x1 + fb.W, Y1: y1, X2: x2, Y2: y2, cols: make([]int, len(chain))}
		for j, r := range chain {
			ep.cols[j] = p.colOf[r.Node]
			if j > 0 && j < len(chain)-1 {
				d := p.boxes[r.Node]
				ep.Via = append(ep.Via, Point{X: d.X, Y: d.Y + d.H/2})
			}
		}
		out = append(out, ep)
	}
	return out
}

func (p *placer) crossings() int { return Crossings(Layout{Edges: p.paths()}) }

func (p *placer) layout() Layout {
	l := Layout{Nodes: make([]NodeBox, 0, len(p.order)), Edges: p.paths()}
	for _, id := range p.order {
		b := p.boxes[id]
		l.Nodes = append(l.Nodes, *b)
		l.Width = max(l.Width, b.X+b.W+margin)
	}
	for _, b := range p.boxes {
		l.Height = max(l.Height, b.Y+b.H+margin)
	}
	return l
}
