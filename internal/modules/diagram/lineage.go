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
	Example    int
}

// Lineage projects mapping-table rows onto a source -> target port graph. Names
// match case-insensitively (first spelling is shown). The label cell holds rule
// codes ("Q-01, C-01"): a full row keeps one direct edge carrying them all; a
// row missing one side routes through one rule pill per code. A target-only row
// with no code but an example value is fed by its own literal pill. Otherwise a
// lone port is only marked unmapped (source) or unsourced (target).
func Lineage(rows [][]string, cols LineageColumns) Graph {
	b := newLineageBuilder()
	for _, row := range rows {
		b.add(
			cell(row, cols.FromEntity), cell(row, cols.FromAttr),
			cell(row, cols.ToEntity), cell(row, cols.ToAttr),
			cell(row, cols.Label), cell(row, cols.Example),
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

// splitCodes splits a rule cell on commas/semicolons, deduped case-insensitively
// with the first spelling kept.
func splitCodes(label string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(label, func(r rune) bool { return r == ',' || r == ';' }) {
		code := strings.TrimSpace(part)
		if code == "" || seen[canon(code)] {
			continue
		}
		seen[canon(code)] = true
		out = append(out, code)
	}
	return out
}

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

// edge dedupes on endpoints + label; the first example note wins.
func (b *lineageBuilder) edge(e Edge) {
	key := Edge{From: e.From, To: e.To, Label: e.Label}
	if b.edgeSet[key] {
		return
	}
	b.edgeSet[key] = true
	b.edges = append(b.edges, e)
	b.fed[e.To] = true
}

func (b *lineageBuilder) add(fe, fa, te, ta, label, example string) {
	from, hasFrom := b.end(LayerSource, fe, fa)
	to, hasTo := b.end(LayerTarget, te, ta)
	codes := splitCodes(label)
	switch {
	case hasFrom && hasTo:
		b.edge(Edge{From: from, To: to, Label: strings.Join(codes, ", "), Note: example})
		b.direct[from] = true
	case hasFrom:
		for _, code := range codes {
			rule := b.node(LayerRule, KindRule, code)
			b.edge(Edge{From: from, To: PortRef{Node: rule.ID}, Label: code, Note: example})
		}
	case hasTo && len(codes) > 0:
		for _, code := range codes {
			rule := b.node(LayerRule, KindRule, code)
			b.edge(Edge{From: PortRef{Node: rule.ID}, To: to, Label: code, Note: example})
		}
	case hasTo && example != "":
		lit := b.literal(to, example)
		b.edge(Edge{From: PortRef{Node: lit.ID}, To: to})
	}
}

// literal is a per-target pill holding a constant; equal values on different
// targets stay separate nodes.
func (b *lineageBuilder) literal(to PortRef, value string) *Node {
	id := "literal:" + to.Node + ":" + to.Port
	if n, ok := b.nodeIdx[id]; ok {
		return n
	}
	n := &Node{ID: id, Label: value, Kind: KindLiteral, Layer: LayerRule, Ports: []Port{}}
	b.nodes = append(b.nodes, n)
	b.nodeIdx[id] = n
	b.hasRule = true
	return n
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
