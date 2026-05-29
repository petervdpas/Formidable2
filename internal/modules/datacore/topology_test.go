package datacore

import "testing"

func graphFixture() *Tensor {
	dt := New()
	// Two records; A has a 2-row items loop and links to B.
	dt.Ingest(Record{
		ID:     "A",
		Fields: map[string]string{"title": "Alpha"},
		Tables: map[string][]map[string]string{"items": {{"name": "disk"}, {"name": "ram"}}},
		Links:  map[string][]string{"owner": {"B"}},
	})
	dt.Ingest(Record{ID: "B", Fields: map[string]string{"title": "Beta"}})
	return dt
}

func nodeKind(g Graph) map[string]string {
	m := map[string]string{}
	for _, n := range g.Nodes {
		m[n.ID] = n.Kind
	}
	return m
}

func TestGraphNodesAndEdges(t *testing.T) {
	g := graphFixture().Graph(0)

	// Nodes: A, B (roots) + 2 item rows (rows). B is reached only by a link
	// from A, but it is its own root.
	if len(g.Nodes) != 4 {
		t.Fatalf("nodes = %d, want 4 (A, B, 2 rows)", len(g.Nodes))
	}
	kinds := nodeKind(g)
	if kinds["A"] != "root" || kinds["B"] != "root" {
		t.Fatalf("A/B kinds = %v, want root", kinds)
	}
	rowCount := 0
	for _, k := range kinds {
		if k == "row" {
			rowCount++
		}
	}
	if rowCount != 2 {
		t.Fatalf("row nodes = %d, want 2", rowCount)
	}

	// Edges: A -> 2 item rows (field items), A -> B (field owner) = 3 edges.
	if len(g.Edges) != 3 {
		t.Fatalf("edges = %d, want 3", len(g.Edges))
	}
	var ownerEdges, itemEdges int
	for _, e := range g.Edges {
		switch e.Field {
		case "owner":
			ownerEdges++
			if e.Source != "A" || e.Target != "B" {
				t.Fatalf("owner edge = %s->%s, want A->B", e.Source, e.Target)
			}
		case "items":
			itemEdges++
			if e.Source != "A" {
				t.Fatalf("item edge source = %s, want A", e.Source)
			}
		}
	}
	if ownerEdges != 1 || itemEdges != 2 {
		t.Fatalf("owner=%d items=%d, want 1 and 2", ownerEdges, itemEdges)
	}
}

func TestGraphCapKeepsRootsFirstAndDropsDanglingEdges(t *testing.T) {
	g := graphFixture().Graph(2)

	if !g.Capped {
		t.Fatal("Capped = false, want true")
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("capped nodes = %d, want 2", len(g.Nodes))
	}
	// Roots come first, so the two kept nodes are A and B.
	kinds := nodeKind(g)
	if kinds["A"] != "root" || kinds["B"] != "root" {
		t.Fatalf("capped kept %v, want roots A and B first", kinds)
	}
	// A -> B survives (both kept); A -> item rows drop (rows excluded).
	if len(g.Edges) != 1 || g.Edges[0].Field != "owner" {
		t.Fatalf("capped edges = %+v, want only owner A->B", g.Edges)
	}
}

func fieldNode(g Graph) (GraphNode, bool) {
	for _, n := range g.Nodes {
		if n.Kind == "field" {
			return n, true
		}
	}
	return GraphNode{}, false
}

func TestGraphFromRootsTheFlower(t *testing.T) {
	dt := graphFixture()

	// Depth 1 from A: A, its title field-node, its 2 item rows, and B.
	g := dt.GraphFrom("A", 1)
	if len(g.Nodes) != 5 {
		t.Fatalf("depth-1 nodes = %d, want 5 (A + title field + 2 rows + B)", len(g.Nodes))
	}
	// Edges: A->title, A->row0, A->row1, A->B.
	if len(g.Edges) != 4 {
		t.Fatalf("depth-1 edges = %d, want 4", len(g.Edges))
	}
	fn, ok := fieldNode(g)
	if !ok || fn.Label != "Alpha" {
		t.Fatalf("field node = %+v, want title value Alpha", fn)
	}

	// Depth 0: the root and its own fields, no ref-children.
	g0 := dt.GraphFrom("A", 0)
	if len(g0.Nodes) != 2 {
		t.Fatalf("depth-0 nodes = %d, want 2 (A + title field)", len(g0.Nodes))
	}

	// Expanding a row reveals that row's columns as field nodes.
	gr := dt.GraphFrom("A#items#0", 1)
	frow, ok := fieldNode(gr)
	if !ok || frow.Label != "disk" {
		t.Fatalf("row column node = %+v, want name value disk", frow)
	}
}

func TestGraphFromUnknownRoot(t *testing.T) {
	if g := graphFixture().GraphFrom("nope", 2); len(g.Nodes) != 0 {
		t.Fatalf("unknown root = %+v, want empty", g)
	}
}

func TestGraphUsesRecordLabel(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "rec1.json", Label: "Quarterly Report", Fields: map[string]string{"x": "1"}})

	g := dt.GraphFrom("rec1.json", 0)
	if g.Nodes[0].ID != "rec1.json" || g.Nodes[0].Label != "Quarterly Report" {
		t.Fatalf("root node = %+v, want id rec1.json label 'Quarterly Report'", g.Nodes[0])
	}
	if g.Nodes[0].Kind != "root" {
		t.Fatalf("root kind = %q, want root", g.Nodes[0].Kind)
	}
}

func TestGraphEmptyTensor(t *testing.T) {
	g := New().Graph(0)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 || g.Capped {
		t.Fatalf("empty graph = %+v, want zero", g)
	}
}
