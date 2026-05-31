package stat

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// scalarQzmForm carries a scalar status field plus an qzm coverage facet, so
// scaling can weight per form on a non-table-column (scalar) dimension. This is
// the scalar counterpart to qzmForm, which uses a table-column dimension.
func scalarQzmForm(file, status, qzm string) index.FormRow {
	r := index.FormRow{
		Template: "samp.yaml",
		Filename: file,
		Mtime:    1,
		Values:   []index.FormValueRow{{FieldKey: "status", ValueType: "text", Text: status}},
	}
	if qzm != "" {
		r.Facets = []index.FormFacet{{Key: "qzm", Set: true, Selected: qzm}}
	}
	return r
}

// pctByLabel reads each axis-0 label's pct for measure 0.
func pctByLabel(g *Grid) map[string]float64 {
	out := map[string]float64{}
	for _, c := range g.Cells {
		if len(c.Pct) > 0 {
			out[g.Axes[0].Labels[c.Coords[0]]] = c.Pct[0]
		}
	}
	return out
}

// TestInteraction_CompositeDrilledChild_PctFormsScaled is the highest-risk
// interaction cell: a composite child whose DSL carries `pct forms scale "..."`.
// It re-enters the historical 153% bug one level deeper (inside the drill). The
// child's weighted pct-forms denominator is the weighted form total over the
// WHOLE template (formCategory is unfiltered), while the numerator is the
// branch-filtered weighted sum, so every drilled cell pct must be <= 100.
func TestInteraction_CompositeDrilledChild_PctFormsScaled(t *testing.T) {
	forms := []index.FormRow{
		formFlagQzm("r1.meta.json", "IN OMLOOP", "NIET ZONNIG", "QMU"),  // factor 4
		formFlagQzm("r2.meta.json", "IN OMLOOP", "NIET ZONNIG", "QMU"),  // factor 4
		formFlagQzm("r3.meta.json", "IN OMLOOP", "ZONNIG", "Bladework"), // factor 1
		formFlagQzm("r4.meta.json", "NIET IN OMLOOP", "ZONNIG", "QMU"),  // out of branch, factor 1
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})
	svc := NewService(m, fakeSource{list: []StatObject{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "apps", DSL: `records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP" pct forms scale "qzm-urgency"`},
		{Name: "qzm-urgency", Scaling: &Scaling{
			Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
			Weights: []WeightEntry{{Label: "ZONNIG", Factor: 1}, {Label: "NIET ZONNIG", Factor: 4}},
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
	// Parent ring: two flag categories, IN OMLOOP drills, sibling stays a leaf.
	if want := []string{"IN OMLOOP", "NIET IN OMLOOP"}; !equalStrs(cg.Parent.Axes[0].Labels, want) {
		t.Fatalf("parent labels = %v, want %v", cg.Parent.Axes[0].Labels, want)
	}
	if len(cg.Branches) != 2 || cg.Branches[1].Child != nil {
		t.Fatalf("branches = %+v, want NIET IN OMLOOP as a leaf", cg.Branches)
	}
	in := cg.Branches[0]
	if in.Branch != "IN OMLOOP" || in.Child == nil {
		t.Fatalf("IN OMLOOP should drill, got %+v", in)
	}

	val := map[string]float64{}
	pct := pctByLabel(in.Child)
	for _, c := range in.Child.Cells {
		val[in.Child.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	// QMU in-branch weighted records: r1 (4) + r2 (4) = 8.
	if val["QMU"] != 8 {
		t.Errorf("drilled QMU weighted records = %v, want 8", val["QMU"])
	}
	// Bladework in-branch weighted records: r3 (1) = 1.
	if val["Bladework"] != 1 {
		t.Errorf("drilled Bladework weighted records = %v, want 1", val["Bladework"])
	}
	// Weighted form total over the whole template: 4+4+1+1 = 10.
	// QMU pct forms = 8/10 = 80; Bladework = 1/10 = 10.
	if !nearly(pct["QMU"], 80) {
		t.Errorf("drilled QMU pct forms = %v, want 80 (weighted denom 10)", pct["QMU"])
	}
	if !nearly(pct["Bladework"], 10) {
		t.Errorf("drilled Bladework pct forms = %v, want 10", pct["Bladework"])
	}
	// The invariant: no drilled cell pct exceeds 100.
	for label, p := range pct {
		if p > 100.0000001 {
			t.Errorf("drilled cell %q pct = %v exceeds 100 (153%% bug regressed inside drill)", label, p)
		}
	}
}

// TestInteraction_ScalarDim_PctFormsScaled pins the 153% guard on the SCALAR
// path. The existing scaling+pct-forms regression is only pinned on the
// table-column/fan dimension; a scalar (one-row-per-form) dimension takes the
// same weighted-denominator code but never fans out, so it must also keep every
// cell pct <= 100 and sum to <= 100.
func TestInteraction_ScalarDim_PctFormsScaled(t *testing.T) {
	forms := []index.FormRow{
		scalarQzmForm("a.meta.json", "active", "NIET ZONNIG"), // factor 4
		scalarQzmForm("b.meta.json", "active", "NIET ZONNIG"), // factor 4
		scalarQzmForm("c.meta.json", "retired", "ZONNIG"),     // factor 1
		scalarQzmForm("d.meta.json", "retired", "ZONNIG"),     // factor 1
	}
	m := NewManager(datacoreBackend(forms))

	cfg, err := Parse(`count() by F["status"] pct forms`)
	if err != nil {
		t.Fatal(err)
	}
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 1}, {Label: "NIET ZONNIG", Factor: 4}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("samp.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}

	val := map[string]float64{}
	for _, c := range g.Cells {
		val[g.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	// active: a (4) + b (4) = 8 weighted count. retired: c (1) + d (1) = 2.
	if val["active"] != 8 {
		t.Errorf("active weighted count = %v, want 8", val["active"])
	}
	if val["retired"] != 2 {
		t.Errorf("retired weighted count = %v, want 2", val["retired"])
	}
	// Weighted form total = 4+4+1+1 = 10. active = 80%, retired = 20%.
	pct := pctByLabel(g)
	if !nearly(pct["active"], 80) {
		t.Errorf("active pct forms = %v, want 80 (scalar weighted denom 10)", pct["active"])
	}
	if !nearly(pct["retired"], 20) {
		t.Errorf("retired pct forms = %v, want 20", pct["retired"])
	}
	// Scalar dim partitions the forms, so pcts sum to exactly 100 here.
	var sum float64
	for _, p := range pct {
		if p > 100.0000001 {
			t.Errorf("scalar cell pct = %v exceeds 100", p)
		}
		sum += p
	}
	if !nearly(sum, 100) {
		t.Errorf("scalar pct sum = %v, want 100 (partitioning dim)", sum)
	}
}

// TestInteraction_ScaledPctForms_BoundOverVariedFactors turns the single
// point-check into a bound: several fixtures with varied factors, asserting
// under scaling+pct-forms EVERY cell pct <= 100 and the (scalar, partitioning)
// category pcts sum to <= 100. A scalar dimension partitions forms, so the
// weighted numerators sum to exactly the weighted denominator; the sum must
// never exceed 100 regardless of the factor spread.
func TestInteraction_ScaledPctForms_BoundOverVariedFactors(t *testing.T) {
	cases := []struct {
		name    string
		forms   []index.FormRow
		weights []WeightEntry
		def     float64
		// wantVal pins each category's weighted count so a wrong-but-still-sums-
		// to-100 weighting cannot pass the bound silently.
		wantVal map[string]float64
		// wantDenom is the weighted form total (the pct-forms denominator).
		wantDenom float64
	}{
		{
			name: "wide-spread",
			forms: []index.FormRow{
				scalarQzmForm("a", "x", "NIET ZONNIG"),
				scalarQzmForm("b", "y", "ZONNIG"),
				scalarQzmForm("c", "z", "DEELS"),
			},
			weights:   []WeightEntry{{Label: "ZONNIG", Factor: 0.25}, {Label: "NIET ZONNIG", Factor: 9}, {Label: "DEELS", Factor: 3}},
			def:       1,
			wantVal:   map[string]float64{"x": 9, "y": 0.25, "z": 3},
			wantDenom: 12.25,
		},
		{
			name: "default-fallthrough",
			forms: []index.FormRow{
				scalarQzmForm("a", "x", ""),            // unset -> default 2
				scalarQzmForm("b", "x", "ZONNIG"),      // 0.5
				scalarQzmForm("c", "y", "NIET ZONNIG"), // 7
				scalarQzmForm("d", "y", ""),            // unset -> default 2
			},
			weights:   []WeightEntry{{Label: "ZONNIG", Factor: 0.5}, {Label: "NIET ZONNIG", Factor: 7}},
			def:       2,
			wantVal:   map[string]float64{"x": 2.5, "y": 9},
			wantDenom: 11.5,
		},
		{
			name: "tiny-factors",
			forms: []index.FormRow{
				scalarQzmForm("a", "x", "ZONNIG"),
				scalarQzmForm("b", "y", "NIET ZONNIG"),
				scalarQzmForm("c", "x", "NIET ZONNIG"),
			},
			weights:   []WeightEntry{{Label: "ZONNIG", Factor: 0.01}, {Label: "NIET ZONNIG", Factor: 0.02}},
			def:       0.5,
			wantVal:   map[string]float64{"x": 0.03, "y": 0.02},
			wantDenom: 0.05,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(datacoreBackend(tc.forms))
			cfg, err := Parse(`count() by F["status"] pct forms`)
			if err != nil {
				t.Fatal(err)
			}
			sc := &Scaling{Source: SourceRef{Kind: SourceFacet, Key: "qzm"}, Weights: tc.weights, Default: tc.def}
			g, err := m.EvaluateScaled("samp.yaml", cfg, sc)
			if err != nil {
				t.Fatal(err)
			}
			// Pin the exact weighted value of every category.
			val := map[string]float64{}
			for _, c := range g.Cells {
				val[g.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
			}
			if len(val) != len(tc.wantVal) {
				t.Fatalf("category count = %d (%v), want %d", len(val), val, len(tc.wantVal))
			}
			for label, want := range tc.wantVal {
				if !nearly(val[label], want) {
					t.Errorf("category %q weighted count = %v, want %v", label, val[label], want)
				}
			}
			// Each category's pct must equal its weighted value over the
			// weighted form total, and never exceed 100.
			pct := pctByLabel(g)
			if len(pct) != len(tc.wantVal) {
				t.Fatalf("pct category count = %d, want %d", len(pct), len(tc.wantVal))
			}
			var sum float64
			for label, p := range pct {
				want := tc.wantVal[label] / tc.wantDenom * 100
				if !nearly(p, want) {
					t.Errorf("category %q pct = %v, want %v (val/denom)", label, p, want)
				}
				if p > 100.0000001 {
					t.Errorf("cell %q pct = %v exceeds 100", label, p)
				}
				sum += p
			}
			// A scalar dim partitions the forms; weighted parts sum to the
			// weighted whole, so the bound is exactly 100 (within rounding).
			if sum > 100.0000001 {
				t.Errorf("category pct sum = %v exceeds 100", sum)
			}
			if !nearly(sum, 100) {
				t.Errorf("category pct sum = %v, want ~100 (partitioning scalar dim)", sum)
			}
		})
	}
}

// TestInteraction_CountScaledPctDistribution exercises count() x scaling x pct
// distribution: the needForms-via-scale-only path (count needs no form set, but
// scaling forces one) with the distribution denominator (sum of weighted cell
// values), so categories sum to 100. Distinct from pct-forms (whole-template
// weighted denominator).
func TestInteraction_CountScaledPctDistribution(t *testing.T) {
	forms := []index.FormRow{
		scalarQzmForm("a.meta.json", "active", "NIET ZONNIG"), // factor 4
		scalarQzmForm("b.meta.json", "active", "ZONNIG"),      // factor 1
		scalarQzmForm("c.meta.json", "retired", "ZONNIG"),     // factor 1
	}
	m := NewManager(datacoreBackend(forms))
	cfg, err := Parse(`count() by F["status"] pct distribution`)
	if err != nil {
		t.Fatal(err)
	}
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 1}, {Label: "NIET ZONNIG", Factor: 4}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("samp.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	val := map[string]float64{}
	for _, c := range g.Cells {
		val[g.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	// active weighted count: a (4) + b (1) = 5. retired: c (1) = 1.
	if val["active"] != 5 {
		t.Errorf("active weighted count = %v, want 5", val["active"])
	}
	if val["retired"] != 1 {
		t.Errorf("retired weighted count = %v, want 1", val["retired"])
	}
	// Distribution denominator = 5+1 = 6. active = 5/6 ~ 83.33, retired ~ 16.67.
	pct := pctByLabel(g)
	if !nearly(pct["active"], 5.0/6.0*100) {
		t.Errorf("active pct distribution = %v, want %v", pct["active"], 5.0/6.0*100)
	}
	if !nearly(pct["retired"], 1.0/6.0*100) {
		t.Errorf("retired pct distribution = %v, want %v", pct["retired"], 1.0/6.0*100)
	}
	// Distribution always sums to exactly 100.
	if !nearly(pct["active"]+pct["retired"], 100) {
		t.Errorf("distribution pct sum = %v, want 100", pct["active"]+pct["retired"])
	}
}

// TestInteraction_FacetDim_PctForms_UnscaledAndScaled crosses a facet
// dimension with pct-forms, both unweighted and weighted. A facet dimension
// partitions forms (one selection per form), so unscaled pct-forms equals
// distribution (categories sum to 100). Under scaling the weighted parts still
// sum to the weighted whole, so the cap holds.
func TestInteraction_FacetDim_PctForms_UnscaledAndScaled(t *testing.T) {
	// flag is the dimension; qzm is the scaling source. Each form carries both.
	forms := []index.FormRow{
		formFlagQzm("r1.meta.json", "IN OMLOOP", "NIET ZONNIG", "QMU"), // flag IN OMLOOP, factor 4
		formFlagQzm("r2.meta.json", "IN OMLOOP", "ZONNIG", "QMU"),      // flag IN OMLOOP, factor 1
		formFlagQzm("r3.meta.json", "NIET IN OMLOOP", "ZONNIG", "QMU"), // flag NIET IN OMLOOP, factor 1
	}
	m := NewManager(datacoreBackend(forms))

	// Unscaled: facet dim partitions the 3 forms. IN OMLOOP = 2/3, NIET = 1/3.
	cfgU, err := Parse(`count() by Facet["flag"] pct forms`)
	if err != nil {
		t.Fatal(err)
	}
	gU, err := m.Evaluate("samp.yaml", cfgU)
	if err != nil {
		t.Fatal(err)
	}
	pctU := pctByLabel(gU)
	if !nearly(pctU["IN OMLOOP"], 2.0/3.0*100) {
		t.Errorf("unscaled IN OMLOOP pct forms = %v, want %v", pctU["IN OMLOOP"], 2.0/3.0*100)
	}
	if !nearly(pctU["NIET IN OMLOOP"], 1.0/3.0*100) {
		t.Errorf("unscaled NIET IN OMLOOP pct forms = %v, want %v", pctU["NIET IN OMLOOP"], 1.0/3.0*100)
	}
	if !nearly(pctU["IN OMLOOP"]+pctU["NIET IN OMLOOP"], 100) {
		t.Errorf("unscaled facet pct sum = %v, want 100", pctU["IN OMLOOP"]+pctU["NIET IN OMLOOP"])
	}

	// Scaled by qzm: weighted form total = 4+1+1 = 6. IN OMLOOP weighted
	// count = r1 (4) + r2 (1) = 5 -> 5/6. NIET = r3 (1) -> 1/6.
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 1}, {Label: "NIET ZONNIG", Factor: 4}},
		Default: 1,
	}
	gS, err := m.EvaluateScaled("samp.yaml", cfgU, sc)
	if err != nil {
		t.Fatal(err)
	}
	valS := map[string]float64{}
	for _, c := range gS.Cells {
		valS[gS.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	if valS["IN OMLOOP"] != 5 {
		t.Errorf("scaled IN OMLOOP weighted count = %v, want 5", valS["IN OMLOOP"])
	}
	pctS := pctByLabel(gS)
	if !nearly(pctS["IN OMLOOP"], 5.0/6.0*100) {
		t.Errorf("scaled IN OMLOOP pct forms = %v, want %v", pctS["IN OMLOOP"], 5.0/6.0*100)
	}
	if !nearly(pctS["NIET IN OMLOOP"], 1.0/6.0*100) {
		t.Errorf("scaled NIET IN OMLOOP pct forms = %v, want %v", pctS["NIET IN OMLOOP"], 1.0/6.0*100)
	}
	var sum float64
	for _, p := range pctS {
		if p > 100.0000001 {
			t.Errorf("scaled facet cell pct = %v exceeds 100", p)
		}
		sum += p
	}
	if !nearly(sum, 100) {
		t.Errorf("scaled facet pct sum = %v, want 100", sum)
	}
}

// TestInteraction_CompositeChild_NumericReduceUnderFacetBranch composes a
// numeric reduce (avg) child under a facet branch. count()/records() on a
// table-column dim are allowed; a numeric reduce on a SCALAR field is also
// allowed (the table-column-dim restriction does not apply). This asserts the
// engine accepts the combination and computes the branch-scoped avg, not a
// rejection.
func TestInteraction_CompositeChild_NumericReduceUnderFacetBranch(t *testing.T) {
	amount := func(file, flag string, v float64) index.FormRow {
		r := index.FormRow{
			Template: "samp.yaml",
			Filename: file,
			Mtime:    1,
			Values:   []index.FormValueRow{{FieldKey: "amount", ValueType: "number", Num: &v}},
		}
		r.Facets = []index.FormFacet{{Key: "flag", Set: true, Selected: flag}}
		return r
	}
	forms := []index.FormRow{
		amount("r1.meta.json", "IN OMLOOP", 10),
		amount("r2.meta.json", "IN OMLOOP", 30),
		amount("r3.meta.json", "NIET IN OMLOOP", 100),
	}
	m := NewManager(datacoreBackend(forms))

	// Child: avg(amount) grouped by status, filtered to the IN OMLOOP branch.
	// A scalar numeric reduce under a branch filter is permitted.
	avgChild := StatConfig{
		Measures:   []Measure{{Op: OpAvg, Source: &SourceRef{Kind: SourceField, Key: "amount"}}},
		Dimensions: []Dimension{{Source: SourceRef{Kind: SourceFacet, Key: "flag"}}},
		Filters:    []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "flag"}, Op: FilterEq, Value: "IN OMLOOP"}},
	}
	cg, err := m.EvaluateComposite("samp.yaml", Composite{
		Parent: flagParent(),
		Edges:  []Edge{{Branch: "IN OMLOOP", Child: avgChild}},
	})
	if err != nil {
		t.Fatalf("numeric-reduce child under a facet branch should be allowed: %v", err)
	}
	// Two facet categories: IN OMLOOP drills, NIET IN OMLOOP is a solid leaf.
	if len(cg.Branches) != 2 {
		t.Fatalf("branches = %d, want 2", len(cg.Branches))
	}
	if cg.Branches[1].Branch != "NIET IN OMLOOP" || cg.Branches[1].Child != nil {
		t.Errorf("NIET IN OMLOOP should be a leaf, got %+v", cg.Branches[1])
	}
	in := cg.Branches[0]
	if in.Branch != "IN OMLOOP" || in.Child == nil {
		t.Fatalf("IN OMLOOP should drill, got %+v", in)
	}
	// The drilled child has exactly one category (the branch), no leakage of a
	// second flag group.
	if len(in.Child.Axes[0].Labels) != 1 || in.Child.Axes[0].Labels[0] != "IN OMLOOP" {
		t.Fatalf("drilled axis = %v, want just [IN OMLOOP]", in.Child.Axes[0].Labels)
	}
	// Only the IN OMLOOP branch's amounts contribute: avg(10, 30) = 20. The
	// out-of-branch 100 must not leak in.
	var avg float64
	for _, c := range in.Child.Cells {
		if in.Child.Axes[0].Labels[c.Coords[0]] == "IN OMLOOP" {
			avg = c.Values[0]
		}
	}
	if avg != 20 {
		t.Errorf("drilled avg(amount) = %v, want 20 (branch-scoped, 100 excluded)", avg)
	}
}

// TestInteraction_FanMultiRow_ScaledPctForms_RecordsVsCount pins the historical
// 153% vector at its source: a form listing the same application on several
// table rows. Under scaling+pct-forms, records() (per distinct form) can never
// exceed the weighted form total, so its pct stays <= 100; count() (per row)
// legitimately may exceed 100 because a multi-row form contributes its factor
// once to the denominator but once per row to count. This pins both arms so a
// regression that conflates the two denominators is caught.
func TestInteraction_FanMultiRow_ScaledPctForms_RecordsVsCount(t *testing.T) {
	forms := []index.FormRow{
		qzmForm("r1.meta.json", "NIET ZONNIG", "QMU", "QMU"), // 2 QMU rows, factor 4
		qzmForm("r2.meta.json", "ZONNIG", "QMU"),             // 1 QMU row, factor 1
		qzmForm("r3.meta.json", "ZONNIG", "Bladework"),       // 1 Bladework row, factor 1
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	// records() then count(), both percented by forms.
	cfg, err := Parse(`records(), count() by F["components"]["item"] pct forms`)
	if err != nil {
		t.Fatal(err)
	}
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 1}, {Label: "NIET ZONNIG", Factor: 4}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("samp.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}

	rec := map[string]float64{}  // records measure (Values[0])
	cnt := map[string]float64{}  // count measure (Values[1])
	recP := map[string]float64{} // records pct (Pct[0])
	cntP := map[string]float64{} // count pct (Pct[1])
	for _, c := range g.Cells {
		label := g.Axes[0].Labels[c.Coords[0]]
		rec[label] = c.Values[0]
		cnt[label] = c.Values[1]
		recP[label] = c.Pct[0]
		cntP[label] = c.Pct[1]
	}
	// Weighted form total = 4+1+1 = 6.
	// records(): QMU distinct forms {r1,r2} = 4+1 = 5; Bladework {r3} = 1.
	if rec["QMU"] != 5 {
		t.Errorf("QMU weighted records = %v, want 5", rec["QMU"])
	}
	if rec["Bladework"] != 1 {
		t.Errorf("Bladework weighted records = %v, want 1", rec["Bladework"])
	}
	// count(): per row. QMU = r1 (2 rows x 4) + r2 (1 row x 1) = 9; Bladework = 1.
	if cnt["QMU"] != 9 {
		t.Errorf("QMU weighted count = %v, want 9 (per row)", cnt["QMU"])
	}
	// records() pct-forms is bounded by 100 (distinct forms <= all forms).
	if !nearly(recP["QMU"], 5.0/6.0*100) {
		t.Errorf("QMU records pct forms = %v, want %v", recP["QMU"], 5.0/6.0*100)
	}
	if recP["QMU"] > 100.0000001 || recP["Bladework"] > 100.0000001 {
		t.Errorf("records pct exceeded 100: QMU=%v Bladework=%v", recP["QMU"], recP["Bladework"])
	}
	// count() pct-forms MAY exceed 100 by design: a multi-row form double-counts
	// in the numerator but counts once in the weighted denominator. QMU = 9/6 = 150.
	if !nearly(cntP["QMU"], 9.0/6.0*100) {
		t.Errorf("QMU count pct forms = %v, want 150 (per-row over weighted forms)", cntP["QMU"])
	}
}
