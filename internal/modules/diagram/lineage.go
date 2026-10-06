package diagram

import "strings"

// LineageColumns names, by position, which table column plays which role. A
// negative or out-of-range index reads as a blank cell.
type LineageColumns struct {
	FromEntity int
	FromAttr   int
	ToEntity   int
	ToAttr     int
	Label      int
}

// Lineage projects mapping-table rows onto a source -> target port graph. Names
// match case-insensitively (first spelling is shown). A row missing one side
// routes through a rule node named by its label, or, with no label, only marks
// the lone port as unmapped (source) or unsourced (target).
func Lineage(rows [][]string, cols LineageColumns) Graph {
	b := newLineageBuilder()
	for _, row := range rows {
		b.add(
			cell(row, cols.FromEntity), cell(row, cols.FromAttr),
			cell(row, cols.ToEntity), cell(row, cols.ToAttr),
			cell(row, cols.Label),
		)
	}
	return b.graph()
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func canon(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

type lineageBuilder struct {
	nodes    []*Node
	nodeIdx  map[string]*Node
	portSeen map[PortRef]bool
	// fed holds target ports with any inbound edge; direct holds source ports
	// mapped straight onto a target (a rule-only source is a join/filter input).
	fed     map[PortRef]bool
	direct  map[PortRef]bool
	edges   []Edge
	edgeSet map[Edge]bool
	hasRule bool
}

func newLineageBuilder() *lineageBuilder {
	return &lineageBuilder{
		nodeIdx:  map[string]*Node{},
		portSeen: map[PortRef]bool{},
		fed:      map[PortRef]bool{},
		direct:   map[PortRef]bool{},
		edgeSet:  map[Edge]bool{},
	}
}

func (b *lineageBuilder) node(layer, kind, label string) *Node {
	id := layer + ":" + canon(label)
	if n, ok := b.nodeIdx[id]; ok {
		return n
	}
	n := &Node{ID: id, Label: label, Kind: kind, Layer: layer, Ports: []Port{}}
	b.nodes = append(b.nodes, n)
	b.nodeIdx[id] = n
	if kind == KindRule {
		b.hasRule = true
	}
	return n
}

// end resolves one side of a row to a PortRef; ok is false when the side is blank.
func (b *lineageBuilder) end(layer, entity, attr string) (PortRef, bool) {
	if entity == "" && attr == "" {
		return PortRef{}, false
	}
	n := b.node(layer, KindEntity, entity)
	if attr == "" {
		return PortRef{Node: n.ID}, true
	}
	ref := PortRef{Node: n.ID, Port: canon(attr)}
	if !b.portSeen[ref] {
		b.portSeen[ref] = true
		n.Ports = append(n.Ports, Port{ID: ref.Port, Label: attr})
	}
	return ref, true
}

func (b *lineageBuilder) edge(e Edge) {
	if b.edgeSet[e] {
		return
	}
	b.edgeSet[e] = true
	b.edges = append(b.edges, e)
	b.fed[e.To] = true
}

func (b *lineageBuilder) add(fe, fa, te, ta, label string) {
	from, hasFrom := b.end(LayerSource, fe, fa)
	to, hasTo := b.end(LayerTarget, te, ta)
	switch {
	case hasFrom && hasTo:
		b.edge(Edge{From: from, To: to, Label: label})
		b.direct[from] = true
	case hasFrom && label != "":
		rule := b.node(LayerRule, KindRule, label)
		b.edge(Edge{From: from, To: PortRef{Node: rule.ID}, Label: label})
	case hasTo && label != "":
		rule := b.node(LayerRule, KindRule, label)
		b.edge(Edge{From: PortRef{Node: rule.ID}, To: to, Label: label})
	}
}

func (b *lineageBuilder) graph() Graph {
	g := Graph{Layers: []Layer{{ID: LayerSource}}, Nodes: []Node{}, Edges: b.edges}
	if b.hasRule {
		g.Layers = append(g.Layers, Layer{ID: LayerRule})
	}
	g.Layers = append(g.Layers, Layer{ID: LayerTarget})
	for _, n := range b.nodes {
		for i := range n.Ports {
			ref := PortRef{Node: n.ID, Port: n.Ports[i].ID}
			switch {
			case n.Layer == LayerSource && !b.direct[ref]:
				n.Ports[i].Class = PortUnmapped
			case n.Layer == LayerTarget && !b.fed[ref]:
				n.Ports[i].Class = PortUnsourced
			}
		}
		g.Nodes = append(g.Nodes, *n)
	}
	if g.Edges == nil {
		g.Edges = []Edge{}
	}
	return g
}
