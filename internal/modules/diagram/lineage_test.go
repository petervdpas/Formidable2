package diagram

import (
	"reflect"
	"testing"
)

// Column order used by these tests: from_entity, from_attr, to_entity, to_attr, label.
var testCols = LineageColumns{FromEntity: 0, FromAttr: 1, ToEntity: 2, ToAttr: 3, Label: 4}

func nodeByID(g Graph, id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

func portLabels(n Node) []string {
	out := make([]string, len(n.Ports))
	for i, p := range n.Ports {
		out[i] = p.Label
	}
	return out
}

func TestLineage_DirectMappingMakesEntitiesPortsAndEdge(t *testing.T) {
	g := Lineage([][]string{{"Basisvariant", "StudielinkVariantCode", "AanmeldingInschrijving", "OpleidingvariantBk", "Q-01"}}, testCols)

	src, ok := nodeByID(g, "source:basisvariant")
	if !ok || src.Label != "Basisvariant" || src.Kind != KindEntity || src.Layer != LayerSource {
		t.Fatalf("source node = %+v (found %v)", src, ok)
	}
	tgt, ok := nodeByID(g, "target:aanmeldinginschrijving")
	if !ok || tgt.Layer != LayerTarget {
		t.Fatalf("target node = %+v (found %v)", tgt, ok)
	}
	if !reflect.DeepEqual(portLabels(src), []string{"StudielinkVariantCode"}) {
		t.Fatalf("source ports = %v", portLabels(src))
	}
	want := Edge{
		From:  PortRef{Node: "source:basisvariant", Port: "studielinkvariantcode"},
		To:    PortRef{Node: "target:aanmeldinginschrijving", Port: "opleidingvariantbk"},
		Label: "Q-01",
	}
	if len(g.Edges) != 1 || g.Edges[0] != want {
		t.Fatalf("edges = %+v, want [%+v]", g.Edges, want)
	}
	if got := layerIDs(g); !reflect.DeepEqual(got, []string{LayerSource, LayerTarget}) {
		t.Fatalf("layers = %v, want source,target (no rule layer without rule nodes)", got)
	}
}

func layerIDs(g Graph) []string {
	out := make([]string, len(g.Layers))
	for i, l := range g.Layers {
		out[i] = l.ID
	}
	return out
}

func TestLineage_NamesMatchCaseInsensitiveFirstSpellingWins(t *testing.T) {
	g := Lineage([][]string{
		{"BasisSoort", "Studielinkcode", "Inschrijving", "Bk", ""},
		{" basissoort ", "STUDIELINKCODE", "inschrijving", "bk", ""},
	}, testCols)
	if len(g.Nodes) != 2 {
		t.Fatalf("nodes = %+v, want 2 (case/space variants collapse)", g.Nodes)
	}
	src, _ := nodeByID(g, "source:basissoort")
	if src.Label != "BasisSoort" || len(src.Ports) != 1 || src.Ports[0].Label != "Studielinkcode" {
		t.Fatalf("source = %+v, want first spelling kept", src)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("edges = %+v, want duplicate row deduped", g.Edges)
	}
}

func TestLineage_SameNameOnBothSidesStaysTwoNodes(t *testing.T) {
	g := Lineage([][]string{{"Student", "Id", "Student", "Bk", ""}}, testCols)
	if _, ok := nodeByID(g, "source:student"); !ok {
		t.Fatal("missing source:student")
	}
	if _, ok := nodeByID(g, "target:student"); !ok {
		t.Fatal("missing target:student")
	}
}

func TestLineage_TargetWithoutSourceButWithRuleFeedsFromRuleNode(t *testing.T) {
	g := Lineage([][]string{{"", "", "AanmeldingInschrijving", "Bk", "Q-01, Q-02"}}, testCols)
	rule, ok := nodeByID(g, "rule:q-01, q-02")
	if !ok || rule.Kind != KindRule || rule.Layer != LayerRule || rule.Label != "Q-01, Q-02" {
		t.Fatalf("rule node = %+v (found %v)", rule, ok)
	}
	want := Edge{
		From:  PortRef{Node: "rule:q-01, q-02"},
		To:    PortRef{Node: "target:aanmeldinginschrijving", Port: "bk"},
		Label: "Q-01, Q-02",
	}
	if len(g.Edges) != 1 || g.Edges[0] != want {
		t.Fatalf("edges = %+v, want [%+v]", g.Edges, want)
	}
	if got := layerIDs(g); !reflect.DeepEqual(got, []string{LayerSource, LayerRule, LayerTarget}) {
		t.Fatalf("layers = %v", got)
	}
}

func TestLineage_SourceWithoutTargetButWithRuleFeedsIntoRuleNode(t *testing.T) {
	g := Lineage([][]string{{"Basisinschrijving", "OpleidingId", "", "", "Q-01"}}, testCols)
	src, _ := nodeByID(g, "source:basisinschrijving")
	if len(src.Ports) != 1 || src.Ports[0].Class != PortUnmapped {
		t.Fatalf("source ports = %+v, want one unmapped port", src.Ports)
	}
	want := Edge{
		From:  PortRef{Node: "source:basisinschrijving", Port: "opleidingid"},
		To:    PortRef{Node: "rule:q-01"},
		Label: "Q-01",
	}
	if len(g.Edges) != 1 || g.Edges[0] != want {
		t.Fatalf("edges = %+v, want [%+v]", g.Edges, want)
	}
}

func TestLineage_UnmappedClassClearsOnceThePortIsMapped(t *testing.T) {
	g := Lineage([][]string{
		{"A", "x", "", "", ""},
		{"A", "x", "B", "y", ""},
	}, testCols)
	src, _ := nodeByID(g, "source:a")
	if src.Ports[0].Class != "" {
		t.Fatalf("port class = %q, want cleared after a real mapping", src.Ports[0].Class)
	}
}

func TestLineage_LoneSidesWithoutRuleAreMarkedNotEdged(t *testing.T) {
	g := Lineage([][]string{
		{"A", "x", "", "", ""},
		{"", "", "B", "y", ""},
	}, testCols)
	if len(g.Edges) != 0 {
		t.Fatalf("edges = %+v, want none", g.Edges)
	}
	a, _ := nodeByID(g, "source:a")
	b, _ := nodeByID(g, "target:b")
	if a.Ports[0].Class != PortUnmapped || b.Ports[0].Class != PortUnsourced {
		t.Fatalf("classes = %q / %q", a.Ports[0].Class, b.Ports[0].Class)
	}
}

func TestLineage_EntityWithoutAttributeAnchorsOnNode(t *testing.T) {
	g := Lineage([][]string{{"A", "", "B", "", ""}}, testCols)
	want := Edge{From: PortRef{Node: "source:a"}, To: PortRef{Node: "target:b"}}
	if len(g.Edges) != 1 || g.Edges[0] != want {
		t.Fatalf("edges = %+v, want [%+v]", g.Edges, want)
	}
}

func TestLineage_AttributeWithoutEntityGoesToUnnamedNode(t *testing.T) {
	g := Lineage([][]string{{"", "x", "B", "y", ""}}, testCols)
	n, ok := nodeByID(g, "source:")
	if !ok || n.Label != "" || len(n.Ports) != 1 {
		t.Fatalf("unnamed source = %+v (found %v)", n, ok)
	}
}

func TestLineage_BlankRowsShortRowsAndAbsentColumnsAreTolerated(t *testing.T) {
	cols := LineageColumns{FromEntity: 0, FromAttr: 1, ToEntity: 2, ToAttr: 3, Label: -1}
	g := Lineage([][]string{
		nil,
		{},
		{"", "", "", ""},
		{"A", "x"},
		{"A", "x", "B", "y", "ignored"},
	}, cols)
	if len(g.Edges) != 1 || g.Edges[0].Label != "" {
		t.Fatalf("edges = %+v, want one unlabelled edge", g.Edges)
	}
}

func TestLineage_OutOfRangeColumnIndexReadsAsBlank(t *testing.T) {
	cols := LineageColumns{FromEntity: 0, FromAttr: 1, ToEntity: 2, ToAttr: 3, Label: 99}
	g := Lineage([][]string{{"A", "x", "B", "y"}}, cols)
	if len(g.Edges) != 1 {
		t.Fatalf("edges = %+v", g.Edges)
	}
}

func TestLineage_EmptyInputYieldsEmptyGraph(t *testing.T) {
	g := Lineage(nil, testCols)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Fatalf("graph = %+v, want empty", g)
	}
}

func TestLineage_NodeAndPortOrderIsFirstSeen(t *testing.T) {
	g := Lineage([][]string{
		{"Z", "b", "T", "q", ""},
		{"A", "a", "T", "p", ""},
		{"Z", "a", "T", "r", ""},
	}, testCols)
	var ids []string
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	if !reflect.DeepEqual(ids, []string{"source:z", "target:t", "source:a"}) {
		t.Fatalf("node order = %v", ids)
	}
	z, _ := nodeByID(g, "source:z")
	if !reflect.DeepEqual(portLabels(z), []string{"b", "a"}) {
		t.Fatalf("port order = %v", portLabels(z))
	}
}
