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

func TestGraphEmptyTensor(t *testing.T) {
	g := New().Graph(0)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 || g.Capped {
		t.Fatalf("empty graph = %+v, want zero", g)
	}
}
