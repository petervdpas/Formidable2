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

func hasNode(g Graph, kind, label string) bool {
	for _, n := range g.Nodes {
		if n.Kind == kind && n.Label == label {
			return true
		}
	}
	return false
}

func kindCount(g Graph, kind string) int {
	n := 0
	for _, node := range g.Nodes {
		if node.Kind == kind {
			n++
		}
	}
	return n
}

func TestGraphFromLevels(t *testing.T) {
	dt := graphFixture()

	// Level 0: the root only.
	g0 := dt.GraphFrom("A", 0)
	if len(g0.Nodes) != 1 || g0.Nodes[0].ID != "A" {
		t.Fatalf("level-0 = %+v, want just A", g0.Nodes)
	}

	// Level 1: root + fields. Scalar "title" as a value node, "items" and
	// "owner" as field nodes, but no rows or linked records yet.
	g1 := dt.GraphFrom("A", 1)
	if !hasNode(g1, "field", "Alpha") || !hasNode(g1, "field", "items") || !hasNode(g1, "field", "owner") {
		t.Fatalf("level-1 fields = %+v", g1.Nodes)
	}
	if kindCount(g1, "row") != 0 {
		t.Fatalf("level-1 rows = %d, want 0 (rows are level 2)", kindCount(g1, "row"))
	}
	if hasNode(g1, "root", "B") {
		t.Fatal("level-1 must not include the linked record B yet")
	}
	// Nodes: A, Alpha, items, owner = 4. Edges: A->Alpha, A->items, A->owner.
	if len(g1.Nodes) != 4 || len(g1.Edges) != 3 {
		t.Fatalf("level-1 = %d nodes, %d edges; want 4 and 3", len(g1.Nodes), len(g1.Edges))
	}

	// Level 2: + the rows under items and the linked record B under owner.
	g2 := dt.GraphFrom("A", 2)
	if kindCount(g2, "row") != 2 {
		t.Fatalf("level-2 rows = %d, want 2", kindCount(g2, "row"))
	}
	if !hasNode(g2, "root", "B") {
		t.Fatal("level-2 missing linked record B")
	}
	if len(g2.Nodes) != 7 || len(g2.Edges) != 6 {
		t.Fatalf("level-2 = %d nodes, %d edges; want 7 and 6", len(g2.Nodes), len(g2.Edges))
	}

	// Expanding a row at level 1 reveals that row's column values.
	gr := dt.GraphFrom("A#items#0", 1)
	if !hasNode(gr, "field", "disk") {
		t.Fatalf("row expand = %+v, want a 'disk' column value node", gr.Nodes)
	}
}

func TestGraphRowNodesAreLabeled(t *testing.T) {
	dt := New()
	dt.Ingest(Record{
		ID:     "rec.json",
		Tables: map[string][]map[string]string{"attributes": {{"name": "Functioneel"}, {"name": "Afgewezen"}}},
		TableLabels: map[string][]string{
			"attributes": {"Functioneel", "Afgewezen"},
		},
	})

	// Rows appear at level 2; they carry their first-column labels.
	g := dt.GraphFrom("rec.json", 2)
	if !hasNode(g, "row", "Functioneel") || !hasNode(g, "row", "Afgewezen") {
		t.Fatalf("row nodes = %+v, want labeled Functioneel + Afgewezen", g.Nodes)
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
