package stat

import (
	"fmt"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// the parent "In Gebruik": count() by Facet["flag"].
func flagParent() StatConfig {
	return StatConfig{
		Measures:   []Measure{{Op: OpCount}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceFacet, Key: "flag"}}},
	}
}

// the child "Applications" scoped to one flag branch.
func appChild(branch string) StatConfig {
	return StatConfig{
		Measures:   []Measure{{Op: OpCount}, {Op: OpRecords}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "code-repositories", Column: "application"}}},
		Filters:    []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "flag"}, Op: FilterEq, Value: branch}},
	}
}

func TestComposite_Validate_AcceptsMatchingFilter(t *testing.T) {
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN GEBRUIK", Child: appChild("IN GEBRUIK")}}}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid composite rejected: %v", err)
	}
}

func TestComposite_Validate_RejectsChildWithoutBranchFilter(t *testing.T) {
	child := appChild("IN GEBRUIK")
	child.Filters = nil // drop the constraint filter
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN GEBRUIK", Child: child}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: child does not filter the base to the branch")
	}
}

func TestComposite_Validate_RejectsFilterOnWrongSource(t *testing.T) {
	child := appChild("IN GEBRUIK")
	child.Filters = []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "fcdm"}, Op: FilterEq, Value: "IN GEBRUIK"}}
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN GEBRUIK", Child: child}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: filter is on a different source than the parent base")
	}
}

func TestComposite_Validate_RejectsFilterWithWrongValue(t *testing.T) {
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN GEBRUIK", Child: appChild("NIET IN GEBRUIK")}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: filter value does not match the branch")
	}
}

func TestComposite_Validate_RejectsNonEqFilter(t *testing.T) {
	child := appChild("IN GEBRUIK")
	child.Filters = []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "flag"}, Op: FilterNe, Value: "IN GEBRUIK"}}
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN GEBRUIK", Child: child}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: branch filter must be eq, not ne")
	}
}

func TestComposite_Validate_RejectsMultiDimParent(t *testing.T) {
	parent := flagParent()
	parent.Dimensions = append(parent.Dimensions, Dimension{Source: SourceRef{Kind: SourceFacet, Key: "fcdm"}})
	c := Composite{Parent: parent, Edges: []Edge{{Branch: "IN GEBRUIK", Child: appChild("IN GEBRUIK")}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: composite parent must be rank-1")
	}
}

func TestComposite_Validate_RejectsDuplicateBranch(t *testing.T) {
	c := Composite{Parent: flagParent(), Edges: []Edge{
		{Branch: "IN GEBRUIK", Child: appChild("IN GEBRUIK")},
		{Branch: "IN GEBRUIK", Child: appChild("IN GEBRUIK")},
	}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: duplicate edge for a branch")
	}
}

// odsFormFlag is odsForm plus a flag facet selection, so a composite can drill
// the flag branch into the application breakdown.
func odsFormFlag(file, flag string, apps ...string) index.FormRow {
	r := odsForm(file, apps...)
	r.Facets = []index.FormFacet{{Key: "flag", Set: true, Selected: flag}}
	return r
}

func TestComposite_Evaluate_DrillsBranchAndLeavesSiblingSolid(t *testing.T) {
	forms := []index.FormRow{
		odsFormFlag("r1.meta.json", "IN GEBRUIK", "FMU", "Gradework"),
		odsFormFlag("r2.meta.json", "IN GEBRUIK", "FMU"),
		odsFormFlag("r3.meta.json", "NIET IN GEBRUIK", "FMU"),
	}
	m := NewManager(realIndex(t, forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})

	cg, err := m.EvaluateComposite("ods.yaml", Composite{
		Parent: flagParent(),
		Edges:  []Edge{{Branch: "IN GEBRUIK", Child: appChild("IN GEBRUIK")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Parent: two facet categories (alphabetical), IN GEBRUIK = 2 records,
	// NIET IN GEBRUIK = 1.
	if want := []string{"IN GEBRUIK", "NIET IN GEBRUIK"}; !equalStrs(cg.Parent.Axes[0].Labels, want) {
		t.Fatalf("parent labels = %v, want %v", cg.Parent.Axes[0].Labels, want)
	}
	if len(cg.Branches) != 2 {
		t.Fatalf("branches = %d, want 2", len(cg.Branches))
	}

	// NIET IN GEBRUIK has no edge -> solid leaf.
	if cg.Branches[1].Branch != "NIET IN GEBRUIK" || cg.Branches[1].Child != nil {
		t.Errorf("NIET IN GEBRUIK should be a leaf, got %+v", cg.Branches[1])
	}

	// IN GEBRUIK drills into the application breakdown over r1+r2 only:
	// FMU 2 mentions / 2 records, Gradework 1 / 1.
	in := cg.Branches[0]
	if in.Branch != "IN GEBRUIK" || in.Child == nil {
		t.Fatalf("IN GEBRUIK should drill, got %+v", in)
	}
	got := map[string][2]float64{}
	for _, c := range in.Child.Cells {
		got[in.Child.Axes[0].Labels[c.Coords[0]]] = [2]float64{c.Values[0], c.Values[1]}
	}
	if v := got["FMU"]; v[0] != 2 || v[1] != 2 {
		t.Errorf("FMU in-use = %v, want [2 2]", v)
	}
	if v := got["Gradework"]; v[0] != 1 || v[1] != 1 {
		t.Errorf("Gradework in-use = %v, want [1 1]", v)
	}
	// The retired-only record (r3 FMU) must not leak into the in-use drill:
	// FMU's in-use records (2) is less than its global mentions (3).
	if got["FMU"][1] >= 3 {
		t.Errorf("FMU in-use records %v leaked the retired record", got["FMU"][1])
	}
}

// ── Discovery: which composites the structure enables ────────────────

func TestCompositeOptions_PairsParentWithFilteredChild(t *testing.T) {
	objs := []StatObject{
		{Name: "fcdm-covered", DSL: `count() by Facet["fcdm"]`},
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "applications", DSL: `count(), records() by F["code-repositories"]["application"] top 10 where Facet["flag"] eq "IN GEBRUIK"`},
	}
	opts := CompositeOptions(objs)

	// in-use is the only parent with an eligible child. applications is also
	// rank-1 but nothing filters its base, so it yields no option.
	if len(opts) != 1 {
		t.Fatalf("options = %d (%+v), want 1", len(opts), opts)
	}
	o := opts[0]
	if o.Parent != "in-use" || o.Base != "flag" {
		t.Errorf("option parent/base = %q/%q, want in-use/flag", o.Parent, o.Base)
	}
	if len(o.Edges) != 1 || o.Edges[0].Branch != "IN GEBRUIK" {
		t.Fatalf("edges = %+v, want one branch IN GEBRUIK", o.Edges)
	}
	if len(o.Edges[0].Children) != 1 || o.Edges[0].Children[0] != "applications" {
		t.Errorf("children = %v, want [applications]", o.Edges[0].Children)
	}
}

func TestCompositeOptions_SkipsNonRank1Parents(t *testing.T) {
	objs := []StatObject{
		{Name: "scalar", DSL: `avg(F["amount"])`},                                     // rank-0
		{Name: "cross", DSL: `count() by Facet["flag"], Facet["fcdm"]`},               // rank-2
		{Name: "child", DSL: `count() by F["x"] where Facet["flag"] eq "IN GEBRUIK"`}, // filters flag
	}
	// No rank-1 object groups by flag, so flag has no parent and there is no
	// composite even though a child filters it.
	if opts := CompositeOptions(objs); len(opts) != 0 {
		t.Errorf("options = %+v, want none (no rank-1 flag parent)", opts)
	}
}

func TestCompositeOptions_OmitsParentWithoutEligibleChild(t *testing.T) {
	objs := []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "by-fcdm", DSL: `count() by Facet["fcdm"]`}, // filters nothing
	}
	if opts := CompositeOptions(objs); len(opts) != 0 {
		t.Errorf("options = %+v, want none (no child drills a branch)", opts)
	}
}

func TestCompositeOptions_ChildFilterOnDifferentSourceIneligible(t *testing.T) {
	objs := []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "wrong", DSL: `count() by F["x"] where Facet["fcdm"] eq "AANWEZIG"`}, // filters fcdm, not flag
	}
	if opts := CompositeOptions(objs); len(opts) != 0 {
		t.Errorf("options = %+v, want none (child filters a different base)", opts)
	}
}

func TestCompositeOptions_SkipsUnparseableDSL(t *testing.T) {
	objs := []StatObject{
		{Name: "broken", DSL: `this is not a dsl`},
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "applications", DSL: `count() by F["a"]["b"] where Facet["flag"] eq "IN GEBRUIK"`},
	}
	opts := CompositeOptions(objs)
	if len(opts) != 1 || opts[0].Parent != "in-use" {
		t.Fatalf("options = %+v, want just in-use (broken skipped)", opts)
	}
}

func TestService_CompositeOptions_NoSourceErrors(t *testing.T) {
	svc := NewService(NewManager(&fakeIndex{}), nil)
	if _, err := svc.CompositeOptions("t"); err == nil {
		t.Error("expected error when no statistic source configured")
	}
}

func TestService_CompositeOptions_DelegatesToSource(t *testing.T) {
	svc := NewService(NewManager(&fakeIndex{}), fakeSource{list: []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "applications", DSL: `count() by F["a"]["b"] where Facet["flag"] eq "IN GEBRUIK"`},
	}})
	opts, err := svc.CompositeOptions("t")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 1 || opts[0].Edges[0].Children[0] != "applications" {
		t.Fatalf("composite options = %+v", opts)
	}
}

// ── Stored-kind resolution (names -> configs via ObjectConfigs) ──────

// stubConfigs is a bare ObjectConfigs over a name->config map, decoupled from
// any catalog shape, so ResolveComposite can be tested in isolation.
type stubConfigs map[string]StatConfig

func (s stubConfigs) Config(name string) (StatConfig, error) {
	c, ok := s[name]
	if !ok {
		return StatConfig{}, fmt.Errorf("unknown object %q", name)
	}
	return c, nil
}

func TestResolveComposite_BuildsCompositeFromNames(t *testing.T) {
	src := stubConfigs{"in-use": flagParent(), "applications": appChild("IN GEBRUIK")}
	comp, err := ResolveComposite(CompositeSpec{
		Parent: "in-use",
		Edges:  []CompositeEdgeSpec{{Branch: "IN GEBRUIK", Child: "applications"}},
	}, src)
	if err != nil {
		t.Fatal(err)
	}
	if err := comp.Validate(); err != nil {
		t.Errorf("resolved composite should validate: %v", err)
	}
	if len(comp.Edges) != 1 || comp.Edges[0].Branch != "IN GEBRUIK" {
		t.Errorf("edges = %+v", comp.Edges)
	}
}

func TestResolveComposite_PropagatesUnknownName(t *testing.T) {
	src := stubConfigs{"in-use": flagParent()}
	if _, err := ResolveComposite(CompositeSpec{
		Parent: "in-use",
		Edges:  []CompositeEdgeSpec{{Branch: "IN GEBRUIK", Child: "ghost"}},
	}, src); err == nil {
		t.Error("expected error: child name not resolvable")
	}
}

func TestCatalogConfigs_RejectsNestedComposite(t *testing.T) {
	cat := catalogConfigs{
		"plain": {Name: "plain", DSL: `count() by Facet["flag"]`},
		"comp":  {Name: "comp", Composite: &CompositeSpec{Parent: "plain"}},
	}
	if _, err := cat.Config("comp"); err == nil {
		t.Error("expected error: a composite cannot be nested as a parent/child")
	}
	if _, err := cat.Config("missing"); err == nil {
		t.Error("expected error: unknown object name")
	}
	if _, err := cat.Config("plain"); err != nil {
		t.Errorf("plain object should resolve: %v", err)
	}
}

func TestService_EvaluateComposite_ResolvesStoredComposite(t *testing.T) {
	forms := []index.FormRow{
		odsFormFlag("r1.meta.json", "IN GEBRUIK", "FMU", "Gradework"),
		odsFormFlag("r2.meta.json", "IN GEBRUIK", "FMU"),
		odsFormFlag("r3.meta.json", "NIET IN GEBRUIK", "FMU"),
	}
	m := NewManager(realIndex(t, forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})
	svc := NewService(m, fakeSource{list: []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "applications", DSL: `count(), records() by F["code-repositories"]["application"] where Facet["flag"] eq "IN GEBRUIK"`},
		{Name: "in-use-by-app", Composite: &CompositeSpec{
			Parent: "in-use",
			Edges:  []CompositeEdgeSpec{{Branch: "IN GEBRUIK", Child: "applications"}},
		}},
	}})

	cg, err := svc.EvaluateComposite("ods.yaml", "in-use-by-app")
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Branches) != 2 || cg.Branches[1].Child != nil {
		t.Fatalf("branches = %+v, want NIET IN GEBRUIK as a leaf", cg.Branches)
	}
	in := cg.Branches[0]
	if in.Branch != "IN GEBRUIK" || in.Child == nil {
		t.Fatalf("IN GEBRUIK should drill, got %+v", in)
	}
	recByApp := map[string]float64{}
	for _, c := range in.Child.Cells {
		recByApp[in.Child.Axes[0].Labels[c.Coords[0]]] = c.Values[1] // records measure
	}
	if recByApp["FMU"] != 2 {
		t.Errorf("FMU in-use records = %v, want 2", recByApp["FMU"])
	}
}

func TestService_EvaluateComposite_NotACompositeErrors(t *testing.T) {
	svc := NewService(NewManager(&fakeIndex{}), fakeSource{list: []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
	}})
	if _, err := svc.EvaluateComposite("t", "in-use"); err == nil {
		t.Error("expected error: named object is a plain DSL object, not a composite")
	}
}

func TestService_EvaluateComposite_UnknownNameErrors(t *testing.T) {
	svc := NewService(NewManager(&fakeIndex{}), fakeSource{list: nil})
	if _, err := svc.EvaluateComposite("t", "ghost"); err == nil {
		t.Error("expected error: unknown statistic name")
	}
}

func TestService_EvaluateComposite_NoSourceErrors(t *testing.T) {
	svc := NewService(NewManager(&fakeIndex{}), nil)
	if _, err := svc.EvaluateComposite("t", "x"); err == nil {
		t.Error("expected error when no statistic source configured")
	}
}

func TestComposite_Evaluate_UnknownBranchErrors(t *testing.T) {
	forms := []index.FormRow{odsFormFlag("r1.meta.json", "IN GEBRUIK", "FMU")}
	m := NewManager(realIndex(t, forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})

	_, err := m.EvaluateComposite("ods.yaml", Composite{
		Parent: flagParent(),
		Edges:  []Edge{{Branch: "GHOST", Child: appChild("GHOST")}},
	})
	if err == nil {
		t.Error("expected error: branch is not a parent category")
	}
}
