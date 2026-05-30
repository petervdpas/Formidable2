package app

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/stat"
)

// Layer 2, continued: the higher-level stat builders that sit on top of the
// Index seam. Scaling weights count()/records() by a per-form factor; a
// composite wires a rank-1 parent to per-branch child grids. Both ultimately
// evaluate through EvaluateScaled -> AggregateRaw, so parity of the Index
// adapter should carry up, but the weighted denominator (the 153% bug area) and
// the composite branch wiring each deserve their own engine-parity check.

func tierScaling() *stat.Scaling {
	return &stat.Scaling{
		Source:  stat.SourceRef{Kind: stat.SourceFacet, Key: "tier"},
		Weights: []stat.WeightEntry{{Label: "GOLD", Factor: 1}, {Label: "SILVER", Factor: 2}},
		Default: 0.5,
	}
}

func TestStatScaledParity(t *testing.T) {
	idxMgr, dcMgr := twoStatManagers(t)
	sc := tierScaling()

	dsls := []string{
		`count() by F["status"]`,
		`count() by F["status"] pct forms`, // weighted denominator path
		`count(), records() by Facet["stage"]`,
		`count() by Facet["tier"] pct forms`,
		`count() by F["due"]@month`,
	}
	for _, d := range dsls {
		cfg, err := stat.Parse(d)
		if err != nil {
			t.Fatalf("parse %q: %v", d, err)
		}
		want, errW := idxMgr.EvaluateScaled("basic.yaml", cfg, sc)
		got, errG := dcMgr.EvaluateScaled("basic.yaml", cfg, sc)
		if (errW == nil) != (errG == nil) {
			t.Fatalf("scaled %q: error mismatch index=%v datacore=%v", d, errW, errG)
		}
		if errW != nil {
			continue
		}
		assertGridsEqual(t, "scaled "+d, want, got)
	}
}

func TestStatCompositeParity(t *testing.T) {
	idxMgr, dcMgr := twoStatManagers(t)

	parent, err := stat.Parse(`count() by Facet["tier"]`)
	if err != nil {
		t.Fatalf("parse parent: %v", err)
	}
	goldChild, err := stat.Parse(`count() by F["status"] where Facet["tier"] eq "GOLD"`)
	if err != nil {
		t.Fatalf("parse child: %v", err)
	}

	// Plain composite: one drilled branch (GOLD), the rest solid leaves.
	plain := stat.Composite{Parent: parent, Edges: []stat.Edge{{Branch: "GOLD", Child: goldChild}}}

	// Scaled composite: the same shape with the parent weighted, to exercise
	// ParentScale through both engines.
	scaled := stat.Composite{Parent: parent, ParentScale: tierScaling(), Edges: []stat.Edge{{Branch: "GOLD", Child: goldChild}}}

	for _, tc := range []struct {
		name string
		comp stat.Composite
	}{
		{"plain", plain},
		{"scaled-parent", scaled},
	} {
		want, errW := idxMgr.EvaluateComposite("basic.yaml", tc.comp)
		got, errG := dcMgr.EvaluateComposite("basic.yaml", tc.comp)
		if (errW == nil) != (errG == nil) {
			t.Fatalf("composite %s: error mismatch index=%v datacore=%v", tc.name, errW, errG)
		}
		if errW != nil {
			continue
		}
		assertCompositeEqual(t, tc.name, want, got)
	}
}

func assertCompositeEqual(t *testing.T, label string, want, got *stat.CompositeGrid) {
	t.Helper()
	assertGridsEqual(t, label+" parent", want.Parent, got.Parent)
	if len(want.Branches) != len(got.Branches) {
		t.Fatalf("%s: branch count want=%d got=%d", label, len(want.Branches), len(got.Branches))
	}
	for i := range want.Branches {
		wb, gb := want.Branches[i], got.Branches[i]
		if wb.Branch != gb.Branch {
			t.Fatalf("%s: branch %d label want=%q got=%q", label, i, wb.Branch, gb.Branch)
		}
		if (wb.Child == nil) != (gb.Child == nil) {
			t.Fatalf("%s: branch %q child presence want=%v got=%v", label, wb.Branch, wb.Child != nil, gb.Child != nil)
		}
		if wb.Child != nil {
			assertGridsEqual(t, label+" branch "+wb.Branch, wb.Child, gb.Child)
		}
	}
}
