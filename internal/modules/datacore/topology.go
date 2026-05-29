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
		g.Nodes = append(g.Nodes, GraphNode{ID: t.iax.label(s), Label: t.nodeLabel(s), Kind: kind})
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

// GraphFrom projects the flower around one identity: the start node, its
// value cells as labeled "field" leaf nodes, and the identities it references
// (loop rows, links) followed to depth hops. Field nodes are attached only to
// the start identity, so the caller unfolds the structure a node at a time:
// click a row and GraphFrom on it reveals that row's columns. Returns an empty
// graph if rootID is unknown.
func (t *Tensor) GraphFrom(rootID string, depth int) Graph {
	root, ok := t.iax.lookup(rootID)
	var g Graph
	if !ok {
		return g
	}

	nodeSeen := map[string]bool{}
	addIdentity := func(s sym) {
		id := t.iax.label(s)
		if nodeSeen[id] {
			return
		}
		nodeSeen[id] = true
		kind := "row"
		if t.rootSet[s] {
			kind = "root"
		}
		g.Nodes = append(g.Nodes, GraphNode{ID: id, Label: t.nodeLabel(s), Kind: kind})
	}
	addIdentity(root)

	rootLabel := t.iax.label(root)
	for k := range t.is {
		if t.is[k] != root || t.ref[k] != 0 {
			continue
		}
		field := t.fax.label(t.fs[k])
		fid := rootLabel + "\x1f" + field
		if !nodeSeen[fid] {
			nodeSeen[fid] = true
			g.Nodes = append(g.Nodes, GraphNode{ID: fid, Label: t.val[k], Kind: "field"})
		}
		g.Edges = append(g.Edges, GraphEdge{Source: rootLabel, Target: fid, Field: field})
	}

	included := map[sym]bool{root: true}
	edgeSeen := map[string]bool{}
	type frontier struct {
		id sym
		d  int
	}
	queue := []frontier{{root, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.d >= depth {
			continue
		}
		for k := range t.is {
			if t.is[k] != cur.id || t.ref[k] == 0 {
				continue
			}
			tgt := t.ref[k]
			addIdentity(tgt)
			ek := t.iax.label(cur.id) + "\x1f" + t.iax.label(tgt) + "\x1f" + t.fax.label(t.fs[k])
			if !edgeSeen[ek] {
				edgeSeen[ek] = true
				g.Edges = append(g.Edges, GraphEdge{
					Source: t.iax.label(cur.id),
					Target: t.iax.label(tgt),
					Field:  t.fax.label(t.fs[k]),
				})
			}
			if !included[tgt] {
				included[tgt] = true
				queue = append(queue, frontier{tgt, cur.d + 1})
			}
		}
	}
	return g
}
