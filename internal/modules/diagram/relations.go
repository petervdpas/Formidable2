package diagram

// RelationColumns names, by position, the columns of an entity-relations table.
type RelationColumns struct {
	Side       int
	FromEntity int
	FromAttr   int
	ToEntity   int
	ToAttr     int
	Label      int
}

// Relations projects entity-relation rows onto a graph of their own: the
// entities of each side with their key attributes, joined by relation edges.
func Relations(rows [][]string, cols RelationColumns) Graph {
	g := Graph{Layers: []Layer{{ID: LayerSource}, {ID: LayerTarget}}, Nodes: []Node{}, Edges: []Edge{}}
	AddRelations(&g, rows, cols)
	return g
}

// AddRelations draws entity-relation rows into g. A row relates two entities
// on one side ("source" or "target"), optionally at an attribute each. Entities
// and attributes already in g are reused as they are (names match
// case-insensitively); new ones are appended, new attributes as key ports. Rows
// without a valid side or without both entities, self-relations and repeats
// are skipped.
func AddRelations(g *Graph, rows [][]string, cols RelationColumns) {
	layers := map[string]bool{}
	for _, l := range g.Layers {
		layers[l.ID] = true
	}
	seen := map[Edge]bool{}
	for _, e := range g.Edges {
		seen[Edge{From: e.From, To: e.To, Label: e.Label, Kind: e.Kind}] = true
	}
	for _, row := range rows {
		side := canon(cell(row, cols.Side))
		fe, te := cell(row, cols.FromEntity), cell(row, cols.ToEntity)
		if (side != LayerSource && side != LayerTarget) || fe == "" || te == "" {
			continue
		}
		if !layers[side] {
			g.Layers = append(g.Layers, Layer{ID: side})
			layers[side] = true
		}
		fa, ta := cell(row, cols.FromAttr), cell(row, cols.ToAttr)
		if canon(fe) == canon(te) && canon(fa) == canon(ta) {
			continue
		}
		from := relationEnd(g, side, fe, fa)
		to := relationEnd(g, side, te, ta)
		e := Edge{From: from, To: to, Label: cell(row, cols.Label), Kind: EdgeRelation}
		if seen[e] {
			continue
		}
		seen[e] = true
		g.Edges = append(g.Edges, e)
	}
}

// relationEnd finds or adds the entity (and attribute, if named) of one end.
func relationEnd(g *Graph, side, entity, attr string) PortRef {
	id := side + ":" + canon(entity)
	idx := -1
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		g.Nodes = append(g.Nodes, Node{ID: id, Label: entity, Kind: KindEntity, Layer: side, Ports: []Port{}})
		idx = len(g.Nodes) - 1
	}
	if attr == "" {
		return PortRef{Node: id}
	}
	n := &g.Nodes[idx]
	pid := canon(attr)
	for _, p := range n.Ports {
		if p.ID == pid {
			return PortRef{Node: id, Port: pid}
		}
	}
	n.Ports = append(n.Ports, Port{ID: pid, Label: attr, Class: PortKey})
	return PortRef{Node: id, Port: pid}
}
