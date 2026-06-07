package datacore

import "strconv"

// GraphNode is one identity in the node-link view: a record (root) or a
// sub-identity (a table/loop row or a linked record).
type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // "root" | "row"
}

// GraphEdge is a directed edge from Source to Target labeled by the field
// that carries the ref (the loop field or the link field).
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Field  string `json:"field"`
}

// Graph is the node-link view of the tensor's reference sub-structure:
// nodes are identities, edges are refs. The tensor read as a labeled
// directed graph rather than as a table.
type Graph struct {
	Nodes  []GraphNode `json:"nodes"`
	Edges  []GraphEdge `json:"edges"`
	Capped bool        `json:"capped"`
}

// Graph projects the reference sub-tensor as nodes and edges. Identities
// are ordered roots-first so a cap keeps whole records before their rows;
// an edge is emitted only when both endpoints survive the cap, so there
// are no dangling edges. limit <= 0 means no cap.
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
		if t.isRecord(s) {
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

// GraphFrom projects the subgraph around one identity at a level of detail:
//
//	level 0: the start node only
//	level 1: + its fields (scalar/facet values as leaf nodes, and table/list/
//	         link fields as field nodes), but no rows under the ref fields
//	level 2: + the rows/targets hanging under each reference field
//
// A scalar field is a leaf labeled by its value; a reference field is a node
// labeled by the field name with its rows under it. The dialog drives the
// base level and uses a per-node call to unfold a clicked row or linked
// record further. Empty graph if rootID is unknown.
func (t *Tensor) GraphFrom(rootID string, level int) Graph {
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
		if t.isRecord(s) {
			kind = "root"
		}
		g.Nodes = append(g.Nodes, GraphNode{ID: id, Label: t.nodeLabel(s), Kind: kind})
	}
	addNode := func(id, label, kind string) {
		if nodeSeen[id] {
			return
		}
		nodeSeen[id] = true
		g.Nodes = append(g.Nodes, GraphNode{ID: id, Label: label, Kind: kind})
	}
	addIdentity(root)
	if level < 1 {
		return g
	}
	rootLabel := t.iax.label(root)

	// Group cells by field, preserving first-seen order, so each field
	// becomes one node (a value leaf or a container for refs).
	type cells struct {
		values []string
		refs   []sym
	}
	order := make([]string, 0)
	byField := map[string]*cells{}
	for k := range t.is {
		if t.is[k] != root {
			continue
		}
		f := t.fax.label(t.fs[k])
		fc := byField[f]
		if fc == nil {
			fc = &cells{}
			byField[f] = fc
			order = append(order, f)
		}
		if t.ref[k] == 0 {
			fc.values = append(fc.values, t.val[k])
		} else {
			fc.refs = append(fc.refs, t.ref[k])
		}
	}

	for _, f := range order {
		fc := byField[f]
		fid := rootLabel + "\x1f" + f
		if len(fc.refs) > 0 {
			addNode(fid, f, "field")
			g.Edges = append(g.Edges, GraphEdge{Source: rootLabel, Target: fid, Field: f})
			if level >= 2 {
				for _, tgt := range fc.refs {
					addIdentity(tgt)
					g.Edges = append(g.Edges, GraphEdge{Source: fid, Target: t.iax.label(tgt), Field: ""})
				}
			}
			continue
		}
		for i, v := range fc.values {
			vid := fid
			if len(fc.values) > 1 {
				vid = fid + "\x1f" + strconv.Itoa(i)
			}
			addNode(vid, v, "field")
			g.Edges = append(g.Edges, GraphEdge{Source: rootLabel, Target: vid, Field: f})
		}
	}
	return g
}
