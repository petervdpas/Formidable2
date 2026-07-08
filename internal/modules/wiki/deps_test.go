package wiki

import (
	"errors"
	"reflect"
	"testing"
)

// fakeGraph is an in-memory DependencyGraph: edges maps a template to its direct
// outgoing dependencies; known is the set of templates that exist. A filename
// with an entry in edges is implicitly known too.
type fakeGraph struct {
	edges map[string][]string
	known map[string]bool
	err   error
}

func newFakeGraph(edges map[string][]string, extraKnown ...string) *fakeGraph {
	g := &fakeGraph{edges: edges, known: map[string]bool{}}
	for from, tos := range edges {
		g.known[from] = true
		for _, to := range tos {
			g.known[to] = true
		}
	}
	for _, k := range extraKnown {
		g.known[k] = true
	}
	return g
}

func (g *fakeGraph) DirectDeps(filename string) ([]string, error) {
	if g.err != nil {
		return nil, g.err
	}
	return g.edges[filename], nil
}

func (g *fakeGraph) TemplateExists(filename string) bool { return g.known[filename] }

func TestResolveDeps_NoDependencies(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.yaml": nil})
	res, err := resolveDeps([]string{"a.yaml"}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Required, []string{"a.yaml"}) {
		t.Errorf("Required = %v, want [a.yaml]", res.Required)
	}
	if len(res.Added) != 0 {
		t.Errorf("Added = %v, want empty", res.Added)
	}
}

func TestResolveDeps_DirectDependencyIsAdded(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.yaml": {"b.yaml"}})
	res, err := resolveDeps([]string{"a.yaml"}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Required, []string{"a.yaml", "b.yaml"}) {
		t.Errorf("Required = %v, want [a.yaml b.yaml]", res.Required)
	}
	if !reflect.DeepEqual(res.Added, []string{"b.yaml"}) {
		t.Errorf("Added = %v, want [b.yaml]", res.Added)
	}
	if !reflect.DeepEqual(res.Because["b.yaml"], []string{"a.yaml"}) {
		t.Errorf("Because[b] = %v, want [a.yaml]", res.Because["b.yaml"])
	}
}

func TestResolveDeps_Transitive(t *testing.T) {
	g := newFakeGraph(map[string][]string{
		"a.yaml": {"b.yaml"},
		"b.yaml": {"c.yaml"},
	})
	res, err := resolveDeps([]string{"a.yaml"}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Required, []string{"a.yaml", "b.yaml", "c.yaml"}) {
		t.Errorf("Required = %v, want a,b,c", res.Required)
	}
	if !reflect.DeepEqual(res.Added, []string{"b.yaml", "c.yaml"}) {
		t.Errorf("Added = %v, want b,c", res.Added)
	}
	// c is transitively pulled in by the original pick a, not by b.
	if !reflect.DeepEqual(res.Because["c.yaml"], []string{"a.yaml"}) {
		t.Errorf("Because[c] = %v, want [a.yaml]", res.Because["c.yaml"])
	}
}

func TestResolveDeps_CycleTerminates(t *testing.T) {
	g := newFakeGraph(map[string][]string{
		"a.yaml": {"b.yaml"},
		"b.yaml": {"a.yaml"},
	})
	res, err := resolveDeps([]string{"a.yaml"}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Required, []string{"a.yaml", "b.yaml"}) {
		t.Errorf("Required = %v, want a,b", res.Required)
	}
	if !reflect.DeepEqual(res.Added, []string{"b.yaml"}) {
		t.Errorf("Added = %v, want [b.yaml]", res.Added)
	}
}

func TestResolveDeps_SelfEdgeIgnored(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.yaml": {"a.yaml"}})
	res, err := resolveDeps([]string{"a.yaml"}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Required, []string{"a.yaml"}) {
		t.Errorf("Required = %v, want [a.yaml]", res.Required)
	}
	if len(res.Added) != 0 {
		t.Errorf("Added = %v, want empty", res.Added)
	}
}

func TestResolveDeps_MissingTargetReportedNotFabricated(t *testing.T) {
	// a links to b, but b is not a known template (dangling reference).
	g := &fakeGraph{
		edges: map[string][]string{"a.yaml": {"b.yaml"}},
		known: map[string]bool{"a.yaml": true},
	}
	res, err := resolveDeps([]string{"a.yaml"}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Required, []string{"a.yaml"}) {
		t.Errorf("Required = %v, want [a.yaml] (b dropped)", res.Required)
	}
	if !reflect.DeepEqual(res.Missing, []string{"b.yaml"}) {
		t.Errorf("Missing = %v, want [b.yaml]", res.Missing)
	}
	if len(res.Added) != 0 {
		t.Errorf("Added = %v, want empty", res.Added)
	}
}

func TestResolveDeps_ExplicitPickNeverInAdded(t *testing.T) {
	// Both a and b are explicit picks; b must not be "added" even though a
	// depends on it.
	g := newFakeGraph(map[string][]string{"a.yaml": {"b.yaml"}})
	res, err := resolveDeps([]string{"a.yaml", "b.yaml"}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Required, []string{"a.yaml", "b.yaml"}) {
		t.Errorf("Required = %v, want a,b", res.Required)
	}
	if len(res.Added) != 0 {
		t.Errorf("Added = %v, want empty (both picked)", res.Added)
	}
}

func TestResolveDeps_SharedDependencyAttributedToBothPicks(t *testing.T) {
	// a and b both depend on shared c; c's Because lists both picks, sorted.
	g := newFakeGraph(map[string][]string{
		"a.yaml": {"c.yaml"},
		"b.yaml": {"c.yaml"},
	})
	res, err := resolveDeps([]string{"a.yaml", "b.yaml"}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Added, []string{"c.yaml"}) {
		t.Errorf("Added = %v, want [c.yaml]", res.Added)
	}
	if !reflect.DeepEqual(res.Because["c.yaml"], []string{"a.yaml", "b.yaml"}) {
		t.Errorf("Because[c] = %v, want [a.yaml b.yaml]", res.Because["c.yaml"])
	}
}

func TestResolveDeps_EmptyAndBlankSeeds(t *testing.T) {
	g := newFakeGraph(map[string][]string{})
	res, err := resolveDeps([]string{"", ""}, g)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.Required) != 0 || len(res.Added) != 0 {
		t.Errorf("blank seeds should yield nothing, got Required=%v Added=%v", res.Required, res.Added)
	}
}

func TestResolveDeps_GraphErrorPropagates(t *testing.T) {
	g := &fakeGraph{
		edges: map[string][]string{"a.yaml": {"b.yaml"}},
		known: map[string]bool{"a.yaml": true, "b.yaml": true},
		err:   errors.New("boom"),
	}
	if _, err := resolveDeps([]string{"a.yaml"}, g); err == nil {
		t.Fatal("expected error to propagate")
	}
}

// Handler.Dependencies without a graph is the identity over the picks.
func TestHandlerDependencies_NoGraphIsIdentity(t *testing.T) {
	h := &Handler{}
	res, err := h.Dependencies([]string{"b.yaml", "a.yaml", "a.yaml", ""})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(res.Required, []string{"a.yaml", "b.yaml"}) {
		t.Errorf("Required = %v, want [a.yaml b.yaml]", res.Required)
	}
	if len(res.Added) != 0 {
		t.Errorf("Added = %v, want empty", res.Added)
	}
}

// expandSelections adds auto-included deps with a nil deck slice and leaves the
// caller's own entries untouched.
func TestExpandSelections_AddsDepsWithAllDecks(t *testing.T) {
	h := &Handler{deps: newFakeGraph(map[string][]string{"a.yaml": {"b.yaml"}})}
	got := h.expandSelections(map[string][]string{"a.yaml": {"deck1"}})
	if !reflect.DeepEqual(got["a.yaml"], []string{"deck1"}) {
		t.Errorf("a.yaml decks = %v, want [deck1] (untouched)", got["a.yaml"])
	}
	entry, ok := got["b.yaml"]
	if !ok {
		t.Fatalf("b.yaml not added; got %v", got)
	}
	if entry != nil {
		t.Errorf("b.yaml decks = %v, want nil (all decks)", entry)
	}
}

func TestExpandSelections_NoGraphReturnsInput(t *testing.T) {
	h := &Handler{}
	in := map[string][]string{"a.yaml": nil}
	if got := h.expandSelections(in); !reflect.DeepEqual(got, in) {
		t.Errorf("expandSelections without graph = %v, want %v", got, in)
	}
}
