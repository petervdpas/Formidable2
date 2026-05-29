package datacore

// GraphNode is one identity in the node-link view: a record (root) or a
// sub-identity (a table/loop row, or a linked record).
type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // "root" | "row"
}

// GraphEdge is one reference: a directed edge from Source to Target labeled by
// the field that carries the ref (the loop field, or the link field).
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Field  string `json:"field"`
}

// Graph is the node-link view of the tensor's reference sub-structure: nodes
// are identities, edges are refs. This is the tensor read as a labeled
// directed graph rather than as a table.
type Graph struct {
	Nodes  []GraphNode `json:"nodes"`
	Edges  []GraphEdge `json:"edges"`
	Capped bool        `json:"capped"`
}

// Graph projects the reference sub-tensor as nodes and edges. Identities are
// ordered roots-first so a cap keeps whole records before their rows; an edge
// is emitted only when both endpoints survive the cap, so there are no
// dangling edges. limit <= 0 means no cap.
func (t *Tensor) Graph(limit int) Graph {
	seen := map[sym]bool{}
	order := make([]sym, 0)
	for _, r := range t.rootList {
		if !seen[r] {
			seen[r] = true
			order = append(order, r)
		}
	}
	for _, s := range t.is {
		if !seen[s] {
			seen[s] = true
			order = append(order, s)
		}
	}

	var g Graph
	keep := len(order)
	if limit > 0 && limit < keep {
		keep = limit
		g.Capped = true
	}

	included := make(map[sym]bool, keep)
	for _, s := range order[:keep] {
		included[s] = true
		kind := "row"
		if t.rootSet[s] {
			kind = "root"
		}
		label := t.iax.label(s)
		g.Nodes = append(g.Nodes, GraphNode{ID: label, Label: label, Kind: kind})
	}

	for k := range t.is {
		if t.ref[k] == 0 {
			continue
		}
		src, tgt := t.is[k], t.ref[k]
		if included[src] && included[tgt] {
			g.Edges = append(g.Edges, GraphEdge{
				Source: t.iax.label(src),
				Target: t.iax.label(tgt),
				Field:  t.fax.label(t.fs[k]),
			})
		}
	}
	return g
}
