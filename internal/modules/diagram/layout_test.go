package diagram

import (
	"reflect"
	"testing"
)

func boxByID(l Layout, id string) (NodeBox, bool) {
	for _, n := range l.Nodes {
		if n.Node.ID == id {
			return n, true
		}
	}
	return NodeBox{}, false
}

func boxPortLabels(n NodeBox) []string {
	out := make([]string, len(n.Ports))
	for i, p := range n.Ports {
		out[i] = p.Port.Label
	}
	return out
}

func TestLayout_EmptyGraph(t *testing.T) {
	l := Place(Graph{})
	if len(l.Nodes) != 0 || len(l.Edges) != 0 {
		t.Fatalf("layout = %+v, want empty", l)
	}
}

func TestLayout_ColumnsFollowLayerOrder(t *testing.T) {
	g := Lineage([][]string{
		{"", "", "T", "a", "R"},
		{"S", "x", "T", "b", ""},
	}, testCols)
	l := Place(g)
	s, _ := boxByID(l, "source:s")
	r, _ := boxByID(l, "rule:r")
	tg, _ := boxByID(l, "target:t")
	if !(s.X+s.W < r.X && r.X+r.W < tg.X) {
		t.Fatalf("x order wrong: source %v..%v rule %v..%v target %v", s.X, s.X+s.W, r.X, r.X+r.W, tg.X)
	}
	if l.Width < tg.X+tg.W || l.Height <= 0 {
		t.Fatalf("canvas %vx%v does not contain target box", l.Width, l.Height)
	}
}

func TestLayout_NodesInAColumnDoNotOverlap(t *testing.T) {
	g := Lineage([][]string{
		{"A", "x", "T", "a", ""},
		{"B", "y", "T", "b", ""},
		{"C", "z", "U", "c", ""},
	}, testCols)
	l := Place(g)
	var col []NodeBox
	for _, n := range l.Nodes {
		if n.Node.Layer == LayerSource {
			col = append(col, n)
		}
	}
	for i := range col {
		for j := range col {
			if i != j && col[i].Y < col[j].Y+col[j].H && col[j].Y < col[i].Y+col[i].H {
				t.Fatalf("%s overlaps %s", col[i].Node.ID, col[j].Node.ID)
			}
		}
	}
}

func TestLayout_PortsSitInsideTheirBoxInOrder(t *testing.T) {
	g := Lineage([][]string{{"A", "x", "T", "a", ""}, {"A", "y", "T", "b", ""}}, testCols)
	l := Place(g)
	for _, n := range l.Nodes {
		prev := n.Y
		for _, p := range n.Ports {
			if p.Y <= prev || p.Y >= n.Y+n.H {
				t.Fatalf("%s port %s y=%v outside/out of order (box %v..%v)", n.Node.ID, p.Port.ID, p.Y, n.Y, n.Y+n.H)
			}
			prev = p.Y
		}
	}
}

func TestLayout_PortsReorderToUncrossEdges(t *testing.T) {
	// Target ports first appear as a,b,c but are fed by x,y,z in reverse.
	g := Lineage([][]string{
		{"", "", "T", "a", ""},
		{"", "", "T", "b", ""},
		{"", "", "T", "c", ""},
		{"A", "x", "T", "c", ""},
		{"A", "y", "T", "b", ""},
		{"A", "z", "T", "a", ""},
	}, testCols)
	if n := Crossings(Place(g)); n != 0 {
		t.Fatalf("crossings = %d, want 0", n)
	}
}

func TestLayout_NodesReorderToUncrossEdges(t *testing.T) {
	g := Lineage([][]string{
		{"A", "x", "Q", "q", ""},
		{"B", "y", "P", "p", ""},
		{"", "", "P", "p", ""},
	}, testCols)
	// First-seen target order is Q, P; sources A, B. Edges A->Q and B->P do not
	// cross in that order, so force the bad order by declaring P first.
	g.Nodes[1], g.Nodes[3] = g.Nodes[3], g.Nodes[1]
	l := Place(g)
	if n := Crossings(l); n != 0 {
		t.Fatalf("crossings = %d, want 0", n)
	}
}

func TestLayout_UnavoidableCrossingIsCountedNotHidden(t *testing.T) {
	// K2,2: both sources map to both targets via the same pair of ports.
	g := Lineage([][]string{
		{"A", "x", "T", "a", ""},
		{"A", "x", "T", "b", ""},
		{"A", "y", "T", "a", ""},
		{"A", "y", "T", "b", ""},
	}, testCols)
	if n := Crossings(Place(g)); n != 1 {
		t.Fatalf("crossings = %d, want 1", n)
	}
}

func TestLayout_IsDeterministic(t *testing.T) {
	rows := [][]string{
		{"A", "x", "T", "c", "R1"},
		{"B", "y", "T", "b", ""},
		{"A", "z", "U", "a", ""},
		{"", "", "U", "d", "R2"},
		{"C", "w", "", "", "R2"},
	}
	first := Place(Lineage(rows, testCols))
	for i := range 20 {
		if got := Place(Lineage(rows, testCols)); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differs", i)
		}
	}
}

func TestLayout_WidthGrowsWithLongestLabel(t *testing.T) {
	short := Place(Lineage([][]string{{"A", "x", "T", "a", ""}}, testCols))
	long := Place(Lineage([][]string{{"A", "AVeryLongAttributeNameIndeedYesVeryLong", "T", "a", ""}}, testCols))
	s, _ := boxByID(short, "source:a")
	lg, _ := boxByID(long, "source:a")
	if lg.W <= s.W {
		t.Fatalf("long label width %v <= short %v", lg.W, s.W)
	}
}

func TestLayout_EdgeAnchorsOnPortsAndNodes(t *testing.T) {
	g := Lineage([][]string{{"A", "x", "T", "", ""}}, testCols)
	l := Place(g)
	if len(l.Edges) != 1 {
		t.Fatalf("edges = %+v", l.Edges)
	}
	a, _ := boxByID(l, "source:a")
	tg, _ := boxByID(l, "target:t")
	e := l.Edges[0]
	if e.X1 != a.X+a.W || e.Y1 != a.Ports[0].Y {
		t.Fatalf("from anchor (%v,%v), want port (%v,%v)", e.X1, e.Y1, a.X+a.W, a.Ports[0].Y)
	}
	if e.X2 != tg.X || e.Y2 <= tg.Y || e.Y2 >= tg.Y+tg.H {
		t.Fatalf("to anchor (%v,%v) not on target box", e.X2, e.Y2)
	}
}

func TestLayout_DanglingEdgeIsDropped(t *testing.T) {
	g := Lineage([][]string{{"A", "x", "T", "a", ""}}, testCols)
	g.Edges = append(g.Edges, Edge{From: PortRef{Node: "nope"}, To: PortRef{Node: "target:t", Port: "zz"}})
	l := Place(g)
	if len(l.Edges) != 1 {
		t.Fatalf("edges = %d, want dangling one dropped", len(l.Edges))
	}
}

func TestLayout_NodeInUnknownLayerGoesLast(t *testing.T) {
	g := Graph{
		Layers: []Layer{{ID: "a"}},
		Nodes:  []Node{{ID: "n1", Label: "N1", Layer: "a"}, {ID: "n2", Label: "N2", Layer: "zzz"}},
	}
	l := Place(g)
	n1, _ := boxByID(l, "n1")
	n2, _ := boxByID(l, "n2")
	if n2.X <= n1.X {
		t.Fatalf("unknown-layer node x=%v not right of %v", n2.X, n1.X)
	}
}
