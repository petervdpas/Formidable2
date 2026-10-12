package diagram

import (
	"reflect"
	"strings"
	"testing"
)

// Column order: side, from_entity, from_attr, to_entity, to_attr, label.
var relCols = RelationColumns{Side: 0, FromEntity: 1, FromAttr: 2, ToEntity: 3, ToAttr: 4, Label: 5}

func relEdges(g Graph) []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if e.Kind == EdgeRelation {
			out = append(out, e)
		}
	}
	return out
}

func TestRelations_AloneBuildsEntitiesKeysAndEdges(t *testing.T) {
	g := Relations([][]string{
		{"source", "Inschrijving", "StudentId", "Student", "StudentId", "StudentId"},
		{"target", "RequestContext", "", "Medewerker", "", ""},
	}, relCols)
	if got := layerIDs(g); !reflect.DeepEqual(got, []string{LayerSource, LayerTarget}) {
		t.Fatalf("layers = %v", got)
	}
	ins, ok := nodeByID(g, "source:inschrijving")
	if !ok || ins.Kind != KindEntity || len(ins.Ports) != 1 || ins.Ports[0].Class != PortKey || ins.Ports[0].Label != "StudentId" {
		t.Fatalf("join entity = %+v (found %v)", ins, ok)
	}
	if rc, ok := nodeByID(g, "target:requestcontext"); !ok || rc.Layer != LayerTarget || len(rc.Ports) != 0 {
		t.Fatalf("target entity = %+v (found %v)", rc, ok)
	}
	want := []Edge{
		{From: PortRef{Node: "source:inschrijving", Port: "studentid"}, To: PortRef{Node: "source:student", Port: "studentid"}, Label: "StudentId", Kind: EdgeRelation},
		{From: PortRef{Node: "target:requestcontext"}, To: PortRef{Node: "target:medewerker"}, Kind: EdgeRelation},
	}
	if !reflect.DeepEqual(relEdges(g), want) {
		t.Fatalf("edges = %+v, want %+v", relEdges(g), want)
	}
}

func TestAddRelations_ReusesLineageEntitiesAndPortsWithoutReclassing(t *testing.T) {
	g := Lineage([][]string{
		{"Account", "Pcn", "Medewerker", "Personeelsnummer", ""},
		{"", "", "RequestContext", "WebserviceKey", "M-01"},
	}, testCols)
	nodes := len(g.Nodes)
	AddRelations(&g, [][]string{
		{"TARGET", " requestcontext ", "", "MEDEWERKER", "personeelsnummer", "bevat"},
		{"source", "Account", "AfdelingId", "Afdeling", "Id", ""},
	}, relCols)
	if len(g.Nodes) != nodes+1 {
		t.Fatalf("nodes = %d, want only Afdeling added to %d", len(g.Nodes), nodes)
	}
	med, _ := nodeByID(g, "target:medewerker")
	if len(med.Ports) != 1 || med.Ports[0].Class != "" {
		t.Fatalf("mapped port must stay as it was: %+v", med.Ports)
	}
	acc, _ := nodeByID(g, "source:account")
	if !reflect.DeepEqual(portLabels(acc), []string{"Pcn", "AfdelingId"}) || acc.Ports[1].Class != PortKey {
		t.Fatalf("account ports = %+v, want the join key appended as a key port", acc.Ports)
	}
	afd, ok := nodeByID(g, "source:afdeling")
	if !ok || afd.Label != "Afdeling" {
		t.Fatalf("join-only entity = %+v (found %v)", afd, ok)
	}
	if n := len(relEdges(g)); n != 2 {
		t.Fatalf("relation edges = %d", n)
	}
}

func TestAddRelations_SkipsUnusableRowsAndDuplicates(t *testing.T) {
	g := Relations([][]string{
		nil,
		{"", "A", "", "B", "", ""},
		{"sideways", "A", "", "B", "", ""},
		{"source", "", "x", "B", "", ""},
		{"source", "A", "", "", "y", ""},
		{"source", "A", "k", "A", "k", ""},
		{"source", "A", "", "B", "", "x"},
		{"source", "a", "", "b", "", "x"},
		{"source", "A"},
	}, relCols)
	if n := len(relEdges(g)); n != 1 {
		t.Fatalf("relation edges = %+v, want exactly the one valid, deduped relation", relEdges(g))
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("nodes = %+v, want only A and B", g.Nodes)
	}
}

func TestLayout_RelationsRunOutsideTheirColumn(t *testing.T) {
	g := Lineage([][]string{{"A", "x", "T", "y", ""}}, testCols)
	AddRelations(&g, [][]string{
		{"source", "A", "x", "B", "k", "join"},
		{"target", "T", "", "U", "", ""},
	}, relCols)
	l := Place(g)
	a, _ := boxByID(l, "source:a")
	b, _ := boxByID(l, "source:b")
	tg, _ := boxByID(l, "target:t")
	u, _ := boxByID(l, "target:u")
	var src, tgt *EdgePath
	for i := range l.Edges {
		e := &l.Edges[i]
		if e.Edge.Kind != EdgeRelation {
			continue
		}
		if e.Edge.From.Node == "source:a" {
			src = e
		} else {
			tgt = e
		}
	}
	if src == nil || tgt == nil {
		t.Fatalf("relation paths missing: %+v", l.Edges)
	}
	if src.X1 != a.X || src.X2 != b.X || len(src.Via) != 2 || src.Via[0].X >= min(a.X, b.X) || src.Via[0].X != src.Via[1].X {
		t.Fatalf("source relation must leave the boxes' left edges into a lane left of the column: %+v", src)
	}
	if src.Via[0].X-textW("join") < 0 {
		t.Fatalf("no room left of the lane for its label: lane x %v", src.Via[0].X)
	}
	right := max(tg.X+tg.W, u.X+u.W)
	if tgt.X1 != tg.X+tg.W || tgt.X2 != u.X+u.W || tgt.Via[0].X <= right {
		t.Fatalf("target relation must run right of the column: %+v", tgt)
	}
	if l.Width <= tgt.Via[0].X {
		t.Fatalf("canvas width %v does not contain the right lane at %v", l.Width, tgt.Via[0].X)
	}
	if src.Y1 != a.Ports[0].Y || src.Y2 != b.Ports[0].Y {
		t.Fatalf("relation must anchor on its key ports: %+v", src)
	}
}

func TestLayout_OverlappingRelationsGetSeparateLanes(t *testing.T) {
	g := Relations([][]string{
		{"source", "A", "", "B", "", ""}, // first-seen order A, B, C, D
		{"source", "A", "", "C", "", ""}, // spans B, so it overlaps A-B
		{"source", "C", "", "D", "", ""},
	}, relCols)
	l := Place(g)
	lane := map[string]float64{}
	for _, e := range l.Edges {
		lane[e.Edge.From.Node+">"+e.Edge.To.Node] = e.Via[0].X
	}
	ac, ab := lane["source:a>source:c"], lane["source:a>source:b"]
	if ac == ab {
		t.Fatalf("A-C and A-B overlap vertically and need different lanes (both at %v)", ac)
	}
	if !(ab > ac) {
		t.Fatalf("the shorter relation takes the inner lane: A-B %v, A-C %v", ab, ac)
	}
}

func TestLayout_RelationsDoNotDisturbFlowCrossingsOrOrder(t *testing.T) {
	rows := [][]string{{"A", "x", "T", "y", ""}, {"B", "z", "T", "w", ""}}
	plain := Place(Lineage(rows, testCols))
	g := Lineage(rows, testCols)
	AddRelations(&g, [][]string{{"source", "B", "", "A", "", ""}}, relCols)
	with := Place(g)
	if Crossings(with) != Crossings(plain) {
		t.Fatalf("crossings changed: %d vs %d", Crossings(with), Crossings(plain))
	}
	a, _ := boxByID(with, "source:a")
	b, _ := boxByID(with, "source:b")
	if !(a.Y < b.Y) {
		t.Fatal("entities must keep table order")
	}
}

func TestSVG_DrawsRelationsWithLabelAndNoLegendEntry(t *testing.T) {
	g := Relations([][]string{{"source", "A", "id", "B", "a_id", "Q-join"}}, relCols)
	s := SVG(Place(g))
	wellFormed(t, s)
	if !strings.Contains(s, `data-relation="0"`) || !strings.Contains(s, ">Q-join<") {
		t.Fatalf("relation or its label missing: %s", s)
	}
	if !strings.Contains(s, "<title>A.id - B.a_id (Q-join)</title>") {
		t.Fatalf("tooltip missing: %s", s)
	}
	if strings.Contains(s, "data-legend=") {
		t.Fatal("a relation label is not a rule code and must not create a legend")
	}
	if !strings.Contains(s, `data-class="key"`) {
		t.Fatal("key ports not marked")
	}
}

func TestSVG_RelationsOnlyDiagramOfEmptyTableIsEmpty(t *testing.T) {
	if s := SVG(Place(Relations(nil, relCols))); s != "" {
		t.Fatalf("got %q", s)
	}
}

func TestLayout_RelationLabelRunsAlongItsOwnLane(t *testing.T) {
	g := Relations([][]string{
		{"source", "A", "", "B", "", "inner"},
		{"source", "A", "", "C", "", "outer"},
		{"target", "T", "", "U", "", "right"},
	}, relCols)
	l := Place(g)
	for _, e := range l.Edges {
		laneX := e.Via[0].X
		if e.LabelY != (e.Y1+e.Y2)/2 {
			t.Fatalf("label %q not centred on its bracket", e.Edge.Label)
		}
		// The label hugs its own lane and stays clear of the neighbouring one.
		if d := e.LabelX - laneX; d == 0 || d <= -laneGap || d >= laneGap {
			t.Fatalf("label %q at x=%v is not beside its lane %v", e.Edge.Label, e.LabelX, laneX)
		}
		if e.Edge.From.Node == "target:t" != (e.LabelX > laneX) {
			t.Fatalf("label %q is on the wrong side of its lane (outside the column)", e.Edge.Label)
		}
	}
}

func TestRelationLabel_IsDroppedWhenItOnlyRepeatsBothAttributes(t *testing.T) {
	cases := []struct {
		row  []string
		want string
	}{
		{[]string{"source", "A", "StudentId", "B", "StudentId", "StudentId"}, ""},
		{[]string{"source", "A", "StudentId", "B", "studentid", " STUDENTID "}, ""},
		{[]string{"source", "A", "StudentId", "B", "Id", "StudentId"}, "StudentId"},
		{[]string{"source", "A", "", "B", "", "bevat"}, "bevat"},
		{[]string{"source", "A", "x", "B", "x", "1..*"}, "1..*"},
	}
	for _, c := range cases {
		l := Place(Relations([][]string{c.row}, relCols))
		if got := RelationLabel(l.Edges[0].Edge); got != c.want {
			t.Errorf("%v: label = %q, want %q", c.row, got, c.want)
		}
	}
}

func TestSVG_RelationsAreDistinctAndLabelsFollowTheBracket(t *testing.T) {
	s := SVG(Place(Relations([][]string{
		{"source", "A", "id", "B", "a_id", "join"},
		{"source", "A", "k", "C", "k", "k"},
	}, relCols)))
	wellFormed(t, s)
	if strings.Count(s, `stroke="`+relationInk+`"`) < 2 || !strings.Contains(s, `stroke-width="1.8"`) {
		t.Fatalf("relations must use their own ink and a heavier line: %s", s)
	}
	if strings.Count(s, "<circle ") != 4 {
		t.Fatalf("each relation needs a dot at both ends, got %d circles", strings.Count(s, "<circle "))
	}
	if !strings.Contains(s, `rotate(-90`) || !strings.Contains(s, ">join<") {
		t.Fatalf("label must run along its bracket: %s", s)
	}
	if strings.Contains(s, ">k</text>") && strings.Count(s, ">k<") > 2 {
		t.Fatalf("a label repeating both attribute names must not be drawn: %s", s)
	}
	for _, c := range []string{"C", "E", "F", "M", "Q", "V"} {
		if categoryColours[c] == relationInk {
			t.Fatalf("relation ink clashes with rule category %s", c)
		}
	}
}
