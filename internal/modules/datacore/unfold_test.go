package datacore

import "testing"

// A homogeneous reporting chain: e1 -> e2 -> e3 (the CEO, no outgoing edge).
// "e1's ultimate boss" is stored in no single cell; e1 only knows e2. It
// emerges from unfolding reports_to to the end of the chain.
func reportingChain() *Tensor {
	dt := New()
	dt.Ingest(Record{ID: "e1", Fields: map[string]string{"title": "junior"}, Links: map[string][]string{"reports_to": {"e2"}}})
	dt.Ingest(Record{ID: "e2", Fields: map[string]string{"title": "lead"}, Links: map[string][]string{"reports_to": {"e3"}}})
	dt.Ingest(Record{ID: "e3", Fields: map[string]string{"title": "CEO"}})
	return dt
}

func collect(seq func(yield func(Node) bool)) []Node {
	var out []Node
	for n := range seq {
		out = append(out, n)
	}
	return out
}

func valueOf(dt *Tensor, id, field string) string {
	v, _, _ := dt.at(dt.iax.intern(id), dt.fax.intern(field), dt.max.intern(Universal))
	return v
}

func TestUnfoldOpensTheChainAndAFactEmergesFromThePath(t *testing.T) {
	dt := reportingChain()

	nodes := collect(dt.View().Where("title", func(v string) bool { return v == "junior" }).Unfold("reports_to", 10))
	if len(nodes) != 2 {
		t.Fatalf("reached %d nodes, want 2 (e2, e3)", len(nodes))
	}
	if nodes[0].ID != "e2" || nodes[0].Depth != 1 {
		t.Fatalf("node0 = %+v, want e2 at depth 1", nodes[0])
	}

	// The emergent fact: e1's ultimate boss is the terminal node, the CEO.
	// No cell of e1 holds "CEO"; the path does.
	end := nodes[len(nodes)-1]
	if end.ID != "e3" || end.Depth != 2 {
		t.Fatalf("terminal = %+v, want e3 at depth 2", end)
	}
	wantPath := []string{"e1", "e2", "e3"}
	for i, p := range wantPath {
		if end.Path[i] != p {
			t.Fatalf("path = %v, want %v", end.Path, wantPath)
		}
	}
	if title := valueOf(dt, end.ID, "title"); title != "CEO" {
		t.Fatalf("ultimate boss title = %q, want CEO", title)
	}
	if got := valueOf(dt, "e1", "title"); got == "CEO" {
		t.Fatal("e1 should not directly hold CEO; the fact must be emergent")
	}
}

func TestUnfoldIsBoundedByDepth(t *testing.T) {
	dt := reportingChain()

	nodes := collect(dt.View().Where("title", func(v string) bool { return v == "junior" }).Unfold("reports_to", 1))
	if len(nodes) != 1 || nodes[0].ID != "e2" {
		t.Fatalf("depth-1 unfold = %+v, want only e2", nodes)
	}
}

func TestUnfoldZeroDepthYieldsNothing(t *testing.T) {
	dt := reportingChain()
	if n := collect(dt.View().Unfold("reports_to", 0)); len(n) != 0 {
		t.Fatalf("depth-0 unfold yielded %d, want 0", len(n))
	}
}

// e1 -> e2 -> e3 -> e1: a cycle. The visited guard must terminate the walk
// and never re-yield a seed or a node.
func TestUnfoldTerminatesOnCycle(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "e1", Links: map[string][]string{"reports_to": {"e2"}}})
	dt.Ingest(Record{ID: "e2", Links: map[string][]string{"reports_to": {"e3"}}})
	dt.Ingest(Record{ID: "e3", Links: map[string][]string{"reports_to": {"e1"}}})

	nodes := collect(dt.View().Unfold("reports_to", 100))
	// Seeds are e1,e2,e3 (every identity). All are visited up front, so a
	// cycle reaches no new petal and the walk halts immediately.
	if len(nodes) != 0 {
		t.Fatalf("cyclic unfold over all seeds yielded %d, want 0 (all visited)", len(nodes))
	}
}

func TestUnfoldFromSingleSeedTerminatesOnCycle(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "e1", Links: map[string][]string{"reports_to": {"e2"}}})
	dt.Ingest(Record{ID: "e2", Links: map[string][]string{"reports_to": {"e3"}}})
	dt.Ingest(Record{ID: "e3", Links: map[string][]string{"reports_to": {"e1"}}})

	// Seed only e1: the walk reaches e2, e3, then e3 -> e1 is the visited
	// seed, so it stops. No infinite loop.
	seeded := &Perspective{t: dt, scope: dt.max.intern(Universal), ids: []sym{dt.iax.intern("e1")}}
	got := collect(seeded.Unfold("reports_to", 100))
	ids := map[string]bool{}
	for _, n := range got {
		ids[n.ID] = true
	}
	if len(got) != 2 || !ids["e2"] || !ids["e3"] {
		t.Fatalf("single-seed cyclic unfold = %+v, want {e2, e3} then halt", got)
	}
}

func TestUnfoldIsLazyAndStopsOnBreak(t *testing.T) {
	dt := reportingChain()

	var first Node
	count := 0
	for n := range dt.View().Where("title", func(v string) bool { return v == "junior" }).Unfold("reports_to", 10) {
		first = n
		count++
		break
	}
	if count != 1 || first.ID != "e2" {
		t.Fatalf("lazy break collected %d (first %q), want 1 (e2)", count, first.ID)
	}
}
