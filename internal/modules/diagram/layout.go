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
	partMinW  = 44.0
	partPad   = 9.0
	partGap   = 4.0
	dummyH    = 4.0
	dummyGap  = 8.0
	sweeps    = 12
	unnamedID = "(?)"
	kindDummy = "~dummy"

	// Relation brackets: lanes outside a column, laneGap apart, labelled on
	// their far side.
	laneGap   = 16.0
	labelPad  = 4.0
	labelLine = 14.0
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

// EdgePath runs from (X1,Y1) through Via to (X2,Y2). For a flow edge Via holds
// the lane it takes through each column it skips, so it never runs behind a
// node. For a relation edge (Edge.Kind) Via holds the two corners of its
// bracket: out of one box, along a lane beside the column, into the other.
type EdgePath struct {
	Edge Edge    `json:"edge"`
	X1   float64 `json:"x1"`
	Y1   float64 `json:"y1"`
	X2   float64 `json:"x2"`
	Y2   float64 `json:"y2"`
	Via  []Point `json:"via,omitempty"`
	// LabelX/LabelY anchor a relation's label, which runs along its bracket:
	// centred on the lane at LabelY, just outside it at LabelX.
	LabelX float64 `json:"labelX,omitempty"`
	LabelY float64 `json:"labelY,omitempty"`
	cols   []int
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
// first-seen order). Entities and their ports keep table (first-seen) order;
// pills and pass-through lanes are ordered by barycenter sweeps, keeping the
// ordering with the fewest crossings. Pure and deterministic.
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
	// relations join two nodes of one column; they take no part in ordering.
	relations []Edge
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
		if !p.anchorOK(e.From) || !p.anchorOK(e.To) {
			continue
		}
		if e.Kind == EdgeRelation {
			if p.colOf[e.From.Node] == p.colOf[e.To.Node] {
				p.relations = append(p.relations, e)
			}
			continue
		}
		p.addEdge(e)
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

// PartWidth is the width of one code pill inside a rule set.
func PartWidth(p Part) float64 { return max(partMinW, textW(p.Label)+2*partPad) }

func textW(s string) float64 { return float64(utf8.RuneCountInString(s)) * charW }

func size(b *NodeBox) {
	if b.Node.Kind == KindRuleSet {
		b.W = 0
		for i, p := range b.Node.Parts {
			if i > 0 {
				b.W += partGap
			}
			b.W += PartWidth(p)
		}
		b.H = ruleH
		return
	}
	if b.Node.Kind != KindEntity {
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

// sweep reorders the free columns by the barycenter of their neighbours on one
// side: forward looks left (inbound edges), backward looks right (outbound).
// Entities and their ports keep table (first-seen) order; only columns of
// pills and lanes move.
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
		if b.Node.Kind == KindEntity {
			return
		}
		inCol[b.Node.ID] = true
	}
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
		s := nodeSum[self.Node]
		nodeSum[self.Node] = [2]float64{s[0] + oy, s[1] + 1}
	}
	keys := map[string]float64{}
	for _, b := range col {
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

// bracket is one placed relation: its lane index on one side of its column.
type bracket struct {
	edge   Edge
	col    int
	right  bool
	lo, hi float64
	lane   int
}

// brackets assigns every relation a lane beside its column: the last column's
// run on its right, all others on their left. Shorter relations take the inner
// lanes; two relations share a lane only when their spans do not touch.
func (p *placer) brackets() []bracket {
	last := -1
	for ci, col := range p.cols {
		if len(col) > 0 {
			last = ci
		}
	}
	out := make([]bracket, 0, len(p.relations))
	for _, e := range p.relations {
		_, y1, _ := p.anchor(e.From)
		_, y2, _ := p.anchor(e.To)
		col := p.colOf[e.From.Node]
		out = append(out, bracket{edge: e, col: col, right: col == last && col != 0, lo: min(y1, y2), hi: max(y1, y2)})
	}
	order := make([]int, len(out))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return out[order[a]].hi-out[order[a]].lo < out[order[b]].hi-out[order[b]].lo
	})
	taken := map[int][]int{} // column -> indices already laned
	for _, i := range order {
		b := &out[i]
		for lane := 0; ; lane++ {
			free := true
			for _, j := range taken[b.col] {
				o := out[j]
				if o.lane == lane && b.lo <= o.hi && o.lo <= b.hi {
					free = false
					break
				}
			}
			if free {
				b.lane = lane
				break
			}
		}
		taken[b.col] = append(taken[b.col], i)
	}
	return out
}

// colSpan is a column's horizontal extent over its real boxes.
func (p *placer) colSpan(ci int) (left, right float64) {
	first := true
	for _, b := range p.cols[ci] {
		if b.Node.Kind == kindDummy {
			continue
		}
		if first || b.X < left {
			left = b.X
		}
		if first || b.X+b.W > right {
			right = b.X + b.W
		}
		first = false
	}
	return left, right
}

func (p *placer) layout() Layout {
	brs := p.brackets()
	// Brackets left of the first column need room: shift everything right.
	need := 0.0
	for _, b := range brs {
		if b.col == 0 && !b.right {
			need = max(need, float64(b.lane+1)*laneGap+labelLine)
		}
	}
	if need > 0 {
		for _, b := range p.boxes {
			b.X += need
		}
	}
	l := p.layoutFlow()
	for _, b := range brs {
		left, right := p.colSpan(b.col)
		laneX := left - float64(b.lane+1)*laneGap
		labelX := laneX - labelPad
		if b.right {
			laneX = right + float64(b.lane+1)*laneGap
			labelX = laneX + labelPad
		}
		_, y1, fb := p.anchor(b.edge.From)
		_, y2, tb := p.anchor(b.edge.To)
		ep := EdgePath{Edge: b.edge, X1: fb.X, Y1: y1, X2: tb.X, Y2: y2, Via: []Point{{laneX, y1}, {laneX, y2}},
			LabelX: labelX, LabelY: (y1 + y2) / 2}
		if b.right {
			ep.X1, ep.X2 = fb.X+fb.W, tb.X+tb.W
			l.Width = max(l.Width, laneX+labelLine+margin)
		}
		l.Edges = append(l.Edges, ep)
	}
	return l
}

func (p *placer) layoutFlow() Layout {
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
