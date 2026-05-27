package stat

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// fcdmForm carries Application cells (code-repositories col 0) plus an fcdm
// coverage facet, so a scaling can weight the form by its coverage.
func fcdmForm(file, fcdm string, apps ...string) index.FormRow {
	f := odsForm(file, apps...)
	if fcdm != "" {
		f.Facets = []index.FormFacet{{Key: "fcdm", Set: true, Selected: fcdm}}
	}
	return f
}

// TestEvaluateScaled_RecordsWeightedByFacet is the headline use: rank
// applications by a coverage-weighted heaviness. Low coverage weighs heavier
// (factor 2), high coverage lighter (0.5), and a form with no coverage set
// falls to the default factor (1).
func TestEvaluateScaled_RecordsWeightedByFacet(t *testing.T) {
	forms := []index.FormRow{
		fcdmForm("x.meta.json", "AANWEZIG", "FMU", "FMU"), // high coverage -> 0.5
		fcdmForm("y.meta.json", "NIET AANWEZIG", "FMU"),   // low coverage -> 2
		fcdmForm("z.meta.json", "", "Gradework"),          // unset -> default 1
	}
	m := NewManager(realIndex(t, forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})

	cfg, err := Parse(`records() by F["code-repositories"]["application"] top 10`)
	if err != nil {
		t.Fatal(err)
	}
	sc := &Scaling{
		Source: SourceRef{Kind: SourceFacet, Key: "fcdm"},
		Weights: []WeightEntry{
			{Label: "AANWEZIG", Factor: 0.5},
			{Label: "NIET AANWEZIG", Factor: 2},
		},
		Default: 1,
	}
	g, err := m.EvaluateScaled("ods.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]float64{}
	for _, c := range g.Cells {
		got[g.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	// FMU: distinct forms x (0.5) + y (2) = 2.5
	if v := got["FMU"]; v != 2.5 {
		t.Errorf("FMU weighted records = %v, want 2.5", v)
	}
	// Gradework: form z, fcdm unset -> default 1
	if v := got["Gradework"]; v != 1 {
		t.Errorf("Gradework weighted records = %v, want 1", v)
	}

	// Unweighted records is the plain distinct-form count, for contrast.
	plain, err := m.Evaluate("ods.yaml", cfg)
	if err != nil {
		t.Fatal(err)
	}
	pgot := map[string]float64{}
	for _, c := range plain.Cells {
		pgot[plain.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	if pgot["FMU"] != 2 || pgot["Gradework"] != 1 {
		t.Errorf("unscaled records FMU=%v Gradework=%v, want 2 and 1", pgot["FMU"], pgot["Gradework"])
	}
}

// TestEvaluateScaled_CountWeightedPerRow checks count() scaling sums a factor
// per row (mentions), not per distinct form.
func TestEvaluateScaled_CountWeightedPerRow(t *testing.T) {
	forms := []index.FormRow{
		fcdmForm("x.meta.json", "AANWEZIG", "FMU", "FMU"), // 2 rows x 0.5
		fcdmForm("y.meta.json", "NIET AANWEZIG", "FMU"),   // 1 row x 2
	}
	m := NewManager(realIndex(t, forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})

	cfg, _ := Parse(`count() by F["code-repositories"]["application"] top 10`)
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "fcdm"},
		Weights: []WeightEntry{{Label: "AANWEZIG", Factor: 0.5}, {Label: "NIET AANWEZIG", Factor: 2}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("ods.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	// FMU: 0.5 + 0.5 + 2 = 3
	if g.Cells[0].Values[0] != 3 {
		t.Errorf("FMU weighted count = %v, want 3", g.Cells[0].Values[0])
	}
}

// TestService_EvaluateObject_ResolvesScaleByName is the unified+reusable path:
// a plain object references a separate scaling object by name through its DSL
// scale clause, and the Service resolves it before evaluating.
func TestService_EvaluateObject_ResolvesScaleByName(t *testing.T) {
	forms := []index.FormRow{
		fcdmForm("x.meta.json", "AANWEZIG", "FMU"),      // 0.5
		fcdmForm("y.meta.json", "NIET AANWEZIG", "FMU"), // 2
	}
	m := NewManager(realIndex(t, forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})

	catalog := []StatObject{
		{
			Name: "gas-apps",
			DSL:  `records() by F["code-repositories"]["application"] top 10 scale "fcdm-urgency"`,
		},
		{
			Name: "fcdm-urgency",
			Scaling: &Scaling{
				Source:  SourceRef{Kind: SourceFacet, Key: "fcdm"},
				Weights: []WeightEntry{{Label: "AANWEZIG", Factor: 0.5}, {Label: "NIET AANWEZIG", Factor: 2}},
				Default: 1,
			},
		},
	}
	svc := NewService(m, fakeSource{list: catalog})

	g, err := svc.EvaluateObject("ods.yaml", "gas-apps")
	if err != nil {
		t.Fatal(err)
	}
	// FMU: 0.5 (x) + 2 (y) = 2.5
	if g.Cells[0].Values[0] != 2.5 {
		t.Errorf("FMU weighted records = %v, want 2.5", g.Cells[0].Values[0])
	}

	// Evaluating the scaling object directly is an error (no grid of its own).
	if _, err := svc.EvaluateObject("ods.yaml", "fcdm-urgency"); err == nil {
		t.Error("expected error evaluating a scaling object directly")
	}
	// An unknown scale name surfaces as an error, not a silent unweighted run.
	bad := []StatObject{{Name: "g", DSL: `records() by F["code-repositories"]["application"] scale "ghost"`}}
	svc2 := NewService(m, fakeSource{list: bad})
	if _, err := svc2.EvaluateObject("ods.yaml", "g"); err == nil {
		t.Error("expected error for unknown scale name")
	}
}

// TestEvaluateScaled_PctFormsUsesWeightedDenominator is the regression for the
// apples-to-pears percentage bug: with scaling, the cell values are weighted
// sums, so `pct forms` must divide by the weighted form total, not the raw
// form count (which produced >100% nonsense like 153%).
func TestEvaluateScaled_PctFormsUsesWeightedDenominator(t *testing.T) {
	forms := []index.FormRow{
		fcdmForm("r1.meta.json", "NIET AANWEZIG", "FMU"),  // factor 4
		fcdmForm("r2.meta.json", "NIET AANWEZIG", "FMU"),  // factor 4
		fcdmForm("r3.meta.json", "AANWEZIG", "Gradework"), // factor 1
		fcdmForm("r4.meta.json", "AANWEZIG", "Gradework"), // factor 1
	}
	m := NewManager(realIndex(t, forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})

	cfg, _ := Parse(`records() by F["code-repositories"]["application"] pct forms`)
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "fcdm"},
		Weights: []WeightEntry{{Label: "AANWEZIG", Factor: 1}, {Label: "NIET AANWEZIG", Factor: 4}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("ods.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	// Weighted form total = 4+4+1+1 = 10 (not the raw count 4). FMU weighs 8,
	// so its forms share is 8/10 = 80%, not the broken 8/4 = 200%.
	pct := map[string]float64{}
	val := map[string]float64{}
	for _, c := range g.Cells {
		label := g.Axes[0].Labels[c.Coords[0]]
		val[label] = c.Values[0]
		pct[label] = c.Pct[0]
	}
	if val["FMU"] != 8 {
		t.Errorf("FMU weighted records = %v, want 8", val["FMU"])
	}
	if !nearly(pct["FMU"], 80) {
		t.Errorf("FMU pct forms = %v, want 80 (weighted denominator 10)", pct["FMU"])
	}
	if !nearly(pct["Gradework"], 20) {
		t.Errorf("Gradework pct forms = %v, want 20", pct["Gradework"])
	}
}

// TestEvaluateScaled_RejectsTableColumnSource guards the per-form rule: a
// table-column scaling source has no single per-form weight.
func TestEvaluateScaled_RejectsTableColumnSource(t *testing.T) {
	m := NewManager(realIndex(t, nil))
	cfg, _ := Parse(`records() by F["base-table"]`)
	sc := &Scaling{Source: SourceRef{Kind: SourceField, Key: "code-repositories", Column: "application"}, Default: 1}
	if _, err := m.EvaluateScaled("ods.yaml", cfg, sc); err == nil {
		t.Fatal("expected error for table-column scaling source")
	}
}
