package stat

import (
	"fmt"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// the parent "Active": count() by Facet["flag"].
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
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}}},
		Filters:    []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "flag"}, Op: FilterEq, Value: branch}},
	}
}

func TestComposite_Validate_AcceptsMatchingFilter(t *testing.T) {
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN OMLOOP", Child: appChild("IN OMLOOP")}}}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid composite rejected: %v", err)
	}
}

func TestComposite_Validate_RejectsChildWithoutBranchFilter(t *testing.T) {
	child := appChild("IN OMLOOP")
	child.Filters = nil // drop the constraint filter
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN OMLOOP", Child: child}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: child does not filter the base to the branch")
	}
}

func TestComposite_Validate_RejectsFilterOnWrongSource(t *testing.T) {
	child := appChild("IN OMLOOP")
	child.Filters = []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "qzm"}, Op: FilterEq, Value: "IN OMLOOP"}}
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN OMLOOP", Child: child}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: filter is on a different source than the parent base")
	}
}

func TestComposite_Validate_RejectsFilterWithWrongValue(t *testing.T) {
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN OMLOOP", Child: appChild("NIET IN OMLOOP")}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: filter value does not match the branch")
	}
}

func TestComposite_Validate_RejectsNonEqFilter(t *testing.T) {
	child := appChild("IN OMLOOP")
	child.Filters = []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "flag"}, Op: FilterNe, Value: "IN OMLOOP"}}
	c := Composite{Parent: flagParent(), Edges: []Edge{{Branch: "IN OMLOOP", Child: child}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: branch filter must be eq, not ne")
	}
}

func TestComposite_Validate_RejectsMultiDimParent(t *testing.T) {
	parent := flagParent()
	parent.Dimensions = append(parent.Dimensions, Dimension{Source: SourceRef{Kind: SourceFacet, Key: "qzm"}})
	c := Composite{Parent: parent, Edges: []Edge{{Branch: "IN OMLOOP", Child: appChild("IN OMLOOP")}}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: composite parent must be rank-1")
	}
}

func TestComposite_Validate_RejectsDuplicateBranch(t *testing.T) {
	c := Composite{Parent: flagParent(), Edges: []Edge{
		{Branch: "IN OMLOOP", Child: appChild("IN OMLOOP")},
		{Branch: "IN OMLOOP", Child: appChild("IN OMLOOP")},
	}}
	if err := c.Validate(); err == nil {
		t.Error("expected rejection: duplicate edge for a branch")
	}
}

// sampFormFlag is sampForm plus a flag facet selection, so a composite can drill
// the flag branch into the application breakdown.
func sampFormFlag(file, flag string, apps ...string) index.FormRow {
	r := sampForm(file, apps...)
	r.Facets = []index.FormFacet{{Key: "flag", Set: true, Selected: flag}}
	return r
}

func TestComposite_Evaluate_DrillsBranchAndLeavesSiblingSolid(t *testing.T) {
	forms := []index.FormRow{
		sampFormFlag("r1.meta.json", "IN OMLOOP", "QMU", "Bladework"),
		sampFormFlag("r2.meta.json", "IN OMLOOP", "QMU"),
		sampFormFlag("r3.meta.json", "NIET IN OMLOOP", "QMU"),
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	cg, err := m.EvaluateComposite("samp.yaml", Composite{
		Parent: flagParent(),
		Edges:  []Edge{{Branch: "IN OMLOOP", Child: appChild("IN OMLOOP")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Parent: two facet categories (alphabetical), IN OMLOOP = 2 records,
	// NIET IN OMLOOP = 1.
	if want := []string{"IN OMLOOP", "NIET IN OMLOOP"}; !equalStrs(cg.Parent.Axes[0].Labels, want) {
		t.Fatalf("parent labels = %v, want %v", cg.Parent.Axes[0].Labels, want)
	}
	if len(cg.Branches) != 2 {
		t.Fatalf("branches = %d, want 2", len(cg.Branches))
	}

	// NIET IN OMLOOP has no edge -> solid leaf.
	if cg.Branches[1].Branch != "NIET IN OMLOOP" || cg.Branches[1].Child != nil {
		t.Errorf("NIET IN OMLOOP should be a leaf, got %+v", cg.Branches[1])
	}

	// IN OMLOOP drills into the application breakdown over r1+r2 only:
	// QMU 2 mentions / 2 records, Bladework 1 / 1.
	in := cg.Branches[0]
	if in.Branch != "IN OMLOOP" || in.Child == nil {
		t.Fatalf("IN OMLOOP should drill, got %+v", in)
	}
	got := map[string][2]float64{}
	for _, c := range in.Child.Cells {
		got[in.Child.Axes[0].Labels[c.Coords[0]]] = [2]float64{c.Values[0], c.Values[1]}
	}
	if v := got["QMU"]; v[0] != 2 || v[1] != 2 {
		t.Errorf("QMU in-use = %v, want [2 2]", v)
	}
	if v := got["Bladework"]; v[0] != 1 || v[1] != 1 {
		t.Errorf("Bladework in-use = %v, want [1 1]", v)
	}
	// The retired-only record (r3 QMU) must not leak into the in-use drill:
	// QMU's in-use records (2) is less than its global mentions (3).
	if got["QMU"][1] >= 3 {
		t.Errorf("QMU in-use records %v leaked the retired record", got["QMU"][1])
	}
}

// ── Discovery: which composites the structure enables ────────────────

func TestCompositeOptions_PairsParentWithFilteredChild(t *testing.T) {
	objs := []StatObject{
		{Name: "qzm-covered", DSL: `count() by Facet["qzm"]`},
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "applications", DSL: `count(), records() by F["components"]["item"] top 10 where Facet["flag"] eq "IN OMLOOP"`},
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
	if len(o.Edges) != 1 || o.Edges[0].Branch != "IN OMLOOP" {
		t.Fatalf("edges = %+v, want one branch IN OMLOOP", o.Edges)
	}
	if len(o.Edges[0].Children) != 1 || o.Edges[0].Children[0] != "applications" {
		t.Errorf("children = %v, want [applications]", o.Edges[0].Children)
	}
}

func TestCompositeOptions_SkipsNonRank1Parents(t *testing.T) {
	objs := []StatObject{
		{Name: "scalar", DSL: `avg(F["amount"])`},                                    // rank-0
		{Name: "cross", DSL: `count() by Facet["flag"], Facet["qzm"]`},               // rank-2
		{Name: "child", DSL: `count() by F["x"] where Facet["flag"] eq "IN OMLOOP"`}, // filters flag
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
		{Name: "by-qzm", DSL: `count() by Facet["qzm"]`}, // filters nothing
	}
	if opts := CompositeOptions(objs); len(opts) != 0 {
		t.Errorf("options = %+v, want none (no child drills a branch)", opts)
	}
}

func TestCompositeOptions_ChildFilterOnDifferentSourceIneligible(t *testing.T) {
	objs := []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "wrong", DSL: `count() by F["x"] where Facet["qzm"] eq "ZONNIG"`}, // filters qzm, not flag
	}
	if opts := CompositeOptions(objs); len(opts) != 0 {
		t.Errorf("options = %+v, want none (child filters a different base)", opts)
	}
}

func TestCompositeOptions_SkipsUnparseableDSL(t *testing.T) {
	objs := []StatObject{
		{Name: "broken", DSL: `this is not a dsl`},
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "applications", DSL: `count() by F["a"]["b"] where Facet["flag"] eq "IN OMLOOP"`},
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
		{Name: "applications", DSL: `count() by F["a"]["b"] where Facet["flag"] eq "IN OMLOOP"`},
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
	src := stubConfigs{"in-use": flagParent(), "applications": appChild("IN OMLOOP")}
	comp, err := ResolveComposite(CompositeSpec{
		Parent: "in-use",
		Edges:  []CompositeEdgeSpec{{Branch: "IN OMLOOP", Child: "applications"}},
	}, src)
	if err != nil {
		t.Fatal(err)
	}
	if err := comp.Validate(); err != nil {
		t.Errorf("resolved composite should validate: %v", err)
	}
	if len(comp.Edges) != 1 || comp.Edges[0].Branch != "IN OMLOOP" {
		t.Errorf("edges = %+v", comp.Edges)
	}
}

func TestResolveComposite_PropagatesUnknownName(t *testing.T) {
	src := stubConfigs{"in-use": flagParent()}
	if _, err := ResolveComposite(CompositeSpec{
		Parent: "in-use",
		Edges:  []CompositeEdgeSpec{{Branch: "IN OMLOOP", Child: "ghost"}},
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
		sampFormFlag("r1.meta.json", "IN OMLOOP", "QMU", "Bladework"),
		sampFormFlag("r2.meta.json", "IN OMLOOP", "QMU"),
		sampFormFlag("r3.meta.json", "NIET IN OMLOOP", "QMU"),
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})
	svc := NewService(m, fakeSource{list: []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "applications", DSL: `count(), records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP"`},
		{Name: "in-use-by-app", Composite: &CompositeSpec{
			Parent: "in-use",
			Edges:  []CompositeEdgeSpec{{Branch: "IN OMLOOP", Child: "applications"}},
		}},
	}})

	cg, err := svc.EvaluateComposite("samp.yaml", "in-use-by-app")
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Branches) != 2 || cg.Branches[1].Child != nil {
		t.Fatalf("branches = %+v, want NIET IN OMLOOP as a leaf", cg.Branches)
	}
	in := cg.Branches[0]
	if in.Branch != "IN OMLOOP" || in.Child == nil {
		t.Fatalf("IN OMLOOP should drill, got %+v", in)
	}
	recByApp := map[string]float64{}
	for _, c := range in.Child.Cells {
		recByApp[in.Child.Axes[0].Labels[c.Coords[0]]] = c.Values[1] // records measure
	}
	if recByApp["QMU"] != 2 {
		t.Errorf("QMU in-use records = %v, want 2", recByApp["QMU"])
	}
}

func TestService_EvaluateCompositeSpec_EvaluatesInlineAgainstSavedObjects(t *testing.T) {
	forms := []index.FormRow{
		sampFormFlag("r1.meta.json", "IN OMLOOP", "QMU"),
		sampFormFlag("r2.meta.json", "NIET IN OMLOOP", "QMU"),
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})
	// Only the parent + child are saved; the composite itself is not (the
	// builder is previewing it before save).
	svc := NewService(m, fakeSource{list: []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "applications", DSL: `count(), records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP"`},
	}})

	cg, err := svc.EvaluateCompositeSpec("samp.yaml", CompositeSpec{
		Parent: "in-use",
		Edges:  []CompositeEdgeSpec{{Branch: "IN OMLOOP", Child: "applications"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Branches) != 2 || cg.Branches[0].Branch != "IN OMLOOP" || cg.Branches[0].Child == nil {
		t.Fatalf("composite spec eval = %+v", cg.Branches)
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

// formFlagQzm carries both the flag facet (so a composite parent can split on
// it) and the qzm coverage facet (so a scaling can weight the drilled child).
func formFlagQzm(file, flag, qzm string, apps ...string) index.FormRow {
	r := sampForm(file, apps...)
	r.Facets = []index.FormFacet{
		{Key: "flag", Set: true, Selected: flag},
		{Key: "qzm", Set: true, Selected: qzm},
	}
	return r
}

// TestService_EvaluateComposite_DrilledChildHonorsScale is the regression for
// the v1 boundary: a composite child carrying a `scale "<name>"` clause must be
// weighted when drilled, exactly as it is standalone. Before the fix the child
// evaluated through Manager.Evaluate (scale dropped), so the drilled ring showed
// raw counts while the standalone object showed weighted sums.
func TestService_EvaluateComposite_DrilledChildHonorsScale(t *testing.T) {
	forms := []index.FormRow{
		formFlagQzm("r1.meta.json", "IN OMLOOP", "NIET ZONNIG", "QMU"), // factor 2
		formFlagQzm("r2.meta.json", "IN OMLOOP", "ZONNIG", "QMU"),      // factor 0.5
		formFlagQzm("r3.meta.json", "NIET IN OMLOOP", "ZONNIG", "QMU"), // out of branch
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})
	svc := NewService(m, fakeSource{list: []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "apps", DSL: `records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP" scale "qzm-urgency"`},
		{Name: "qzm-urgency", Scaling: &Scaling{
			Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
			Weights: []WeightEntry{{Label: "ZONNIG", Factor: 0.5}, {Label: "NIET ZONNIG", Factor: 2}},
			Default: 1,
		}},
		{Name: "in-use-by-app", Composite: &CompositeSpec{
			Parent: "in-use",
			Edges:  []CompositeEdgeSpec{{Branch: "IN OMLOOP", Child: "apps"}},
		}},
	}})

	cg, err := svc.EvaluateComposite("samp.yaml", "in-use-by-app")
	if err != nil {
		t.Fatal(err)
	}
	in := cg.Branches[0]
	if in.Branch != "IN OMLOOP" || in.Child == nil {
		t.Fatalf("IN OMLOOP should drill, got %+v", in)
	}
	var alpha float64
	for _, c := range in.Child.Cells {
		if in.Child.Axes[0].Labels[c.Coords[0]] == "QMU" {
			alpha = c.Values[0]
		}
	}
	// QMU across r1 (2) + r2 (0.5) = 2.5 weighted records, not 2 raw distinct forms.
	if alpha != 2.5 {
		t.Errorf("drilled QMU weighted records = %v, want 2.5 (child scale honored)", alpha)
	}
}

// TestComposite_Evaluate_EdgeScaleWeightsChild is the Manager-level counterpart:
// a resolved Edge.Scale weights the drilled child grid.
func TestComposite_Evaluate_EdgeScaleWeightsChild(t *testing.T) {
	forms := []index.FormRow{
		formFlagQzm("r1.meta.json", "IN OMLOOP", "NIET ZONNIG", "QMU"),
		formFlagQzm("r2.meta.json", "IN OMLOOP", "ZONNIG", "QMU"),
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 0.5}, {Label: "NIET ZONNIG", Factor: 2}},
		Default: 1,
	}
	cg, err := m.EvaluateComposite("samp.yaml", Composite{
		Parent: flagParent(),
		Edges:  []Edge{{Branch: "IN OMLOOP", Child: appChild("IN OMLOOP"), Scale: sc}},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := cg.Branches[0]
	if in.Child == nil {
		t.Fatal("IN OMLOOP should drill")
	}
	// appChild measures count() then records(); records is Values[1].
	var alphaRecords float64
	for _, c := range in.Child.Cells {
		if in.Child.Axes[0].Labels[c.Coords[0]] == "QMU" {
			alphaRecords = c.Values[1]
		}
	}
	if alphaRecords != 2.5 {
		t.Errorf("drilled QMU weighted records = %v, want 2.5", alphaRecords)
	}
}

// TestComposite_Evaluate_ParentScaleWeightsRing checks a composite parent's own
// scale clause weights the parent ring, so both branch slices are weighted (not
// just the drilled child). count() weighted sums one factor per form.
func TestComposite_Evaluate_ParentScaleWeightsRing(t *testing.T) {
	forms := []index.FormRow{
		formFlagQzm("r1.meta.json", "IN OMLOOP", "NIET ZONNIG", "QMU"), // factor 2
		formFlagQzm("r2.meta.json", "IN OMLOOP", "ZONNIG", "QMU"),      // factor 0.5
		formFlagQzm("r3.meta.json", "NIET IN OMLOOP", "ZONNIG", "QMU"), // factor 0.5
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 0.5}, {Label: "NIET ZONNIG", Factor: 2}},
		Default: 1,
	}
	cg, err := m.EvaluateComposite("samp.yaml", Composite{
		Parent:      flagParent(),
		ParentScale: sc,
		Edges:       []Edge{{Branch: "IN OMLOOP", Child: appChild("IN OMLOOP"), Scale: sc}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, c := range cg.Parent.Cells {
		got[cg.Parent.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	if got["IN OMLOOP"] != 2.5 { // r1 (2) + r2 (0.5)
		t.Errorf("IN OMLOOP weighted = %v, want 2.5", got["IN OMLOOP"])
	}
	if got["NIET IN OMLOOP"] != 0.5 { // r3 (0.5)
		t.Errorf("NIET IN OMLOOP weighted = %v, want 0.5", got["NIET IN OMLOOP"])
	}
}

// TestResolveComposite_ResolvesChildScale checks the resolver attaches each
// child's weighting to its edge.
func TestResolveComposite_ResolvesChildScale(t *testing.T) {
	child := appChild("IN OMLOOP")
	child.Scale = "qzm-urgency"
	cat := catalogConfigs{
		"in-use":      {Name: "in-use", DSL: `count() by Facet["flag"]`},
		"apps":        {Name: "apps", DSL: compileMust(t, child)},
		"qzm-urgency": {Name: "qzm-urgency", Scaling: &Scaling{Source: SourceRef{Kind: SourceFacet, Key: "qzm"}, Default: 1}},
	}
	comp, err := ResolveComposite(CompositeSpec{
		Parent: "in-use",
		Edges:  []CompositeEdgeSpec{{Branch: "IN OMLOOP", Child: "apps"}},
	}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if len(comp.Edges) != 1 || comp.Edges[0].Scale == nil {
		t.Fatalf("edge scale not resolved: %+v", comp.Edges)
	}
}

// TestResolveComposite_ErrorsWhenSourceCannotResolveScale guards the no-silent-
// drop rule: a child names a scale but the source cannot resolve scalings, so
// resolution errors rather than charting an unweighted child.
func TestResolveComposite_ErrorsWhenSourceCannotResolveScale(t *testing.T) {
	child := appChild("IN OMLOOP")
	child.Scale = "qzm-urgency"
	src := stubConfigs{"in-use": flagParent(), "apps": child}
	if _, err := ResolveComposite(CompositeSpec{
		Parent: "in-use",
		Edges:  []CompositeEdgeSpec{{Branch: "IN OMLOOP", Child: "apps"}},
	}, src); err == nil {
		t.Error("expected error: child has a scale clause but the source cannot resolve scalings")
	}
}

func compileMust(t *testing.T, cfg StatConfig) string {
	t.Helper()
	s, err := Compile(cfg)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return s
}

func TestComposite_Evaluate_UnknownBranchErrors(t *testing.T) {
	forms := []index.FormRow{sampFormFlag("r1.meta.json", "IN OMLOOP", "QMU")}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	_, err := m.EvaluateComposite("samp.yaml", Composite{
		Parent: flagParent(),
		Edges:  []Edge{{Branch: "GHOST", Child: appChild("GHOST")}},
	})
	if err == nil {
		t.Error("expected error: branch is not a parent category")
	}
}
