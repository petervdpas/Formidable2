package stat

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// qzmForm carries Application cells (components col 0) plus an qzm
// coverage facet, so a scaling can weight the form by its coverage.
func qzmForm(file, qzm string, apps ...string) index.FormRow {
	f := sampForm(file, apps...)
	if qzm != "" {
		f.Facets = []index.FormFacet{{Key: "qzm", Set: true, Selected: qzm}}
	}
	return f
}

// TestEvaluateScaled_RecordsWeightedByFacet is the headline use: rank
// applications by a coverage-weighted heaviness. Low coverage weighs heavier
// (factor 2), high coverage lighter (0.5), and a form with no coverage set
// falls to the default factor (1).
func TestEvaluateScaled_RecordsWeightedByFacet(t *testing.T) {
	forms := []index.FormRow{
		qzmForm("x.meta.json", "ZONNIG", "QMU", "QMU"), // high coverage -> 0.5
		qzmForm("y.meta.json", "NIET ZONNIG", "QMU"),   // low coverage -> 2
		qzmForm("z.meta.json", "", "Bladework"),        // unset -> default 1
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	cfg, err := Parse(`records() by F["components"]["item"] top 10`)
	if err != nil {
		t.Fatal(err)
	}
	sc := &Scaling{
		Source: SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{
			{Label: "ZONNIG", Factor: 0.5},
			{Label: "NIET ZONNIG", Factor: 2},
		},
		Default: 1,
	}
	g, err := m.EvaluateScaled("samp.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]float64{}
	for _, c := range g.Cells {
		got[g.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	// QMU: distinct forms x (0.5) + y (2) = 2.5
	if v := got["QMU"]; v != 2.5 {
		t.Errorf("QMU weighted records = %v, want 2.5", v)
	}
	// Bladework: form z, qzm unset -> default 1
	if v := got["Bladework"]; v != 1 {
		t.Errorf("Bladework weighted records = %v, want 1", v)
	}

	// Unweighted records is the plain distinct-form count, for contrast.
	plain, err := m.Evaluate("samp.yaml", cfg)
	if err != nil {
		t.Fatal(err)
	}
	pgot := map[string]float64{}
	for _, c := range plain.Cells {
		pgot[plain.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	if pgot["QMU"] != 2 || pgot["Bladework"] != 1 {
		t.Errorf("unscaled records QMU=%v Bladework=%v, want 2 and 1", pgot["QMU"], pgot["Bladework"])
	}
}

// TestEvaluateScaled_MultipleScalingsMultiply is the two-weight headline:
// each record's factor is the product of two scalings (impact x urgency), so a
// big-impact low-coverage app outscores a small-impact high-coverage one.
func TestEvaluateScaled_MultipleScalingsMultiply(t *testing.T) {
	twoFacet := func(file, tshirt, qzm string, apps ...string) index.FormRow {
		f := sampForm(file, apps...)
		f.Facets = []index.FormFacet{
			{Key: "tshirt", Set: true, Selected: tshirt},
			{Key: "qzm", Set: true, Selected: qzm},
		}
		return f
	}
	forms := []index.FormRow{
		twoFacet("x.meta.json", "L", "NIET ZONNIG", "QMU"), // 3 * 2   = 6
		twoFacet("y.meta.json", "S", "ZONNIG", "QMU"),      // 1 * 0.5 = 0.5
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	cfg, _ := Parse(`records() by F["components"]["item"] top 10`)
	impact := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "tshirt"},
		Weights: []WeightEntry{{Label: "L", Factor: 3}, {Label: "S", Factor: 1}},
		Default: 1,
	}
	urgency := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 0.5}, {Label: "NIET ZONNIG", Factor: 2}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("samp.yaml", cfg, impact, urgency)
	if err != nil {
		t.Fatal(err)
	}
	// QMU: distinct forms x (3*2=6) + y (1*0.5=0.5) = 6.5
	if v := g.Cells[0].Values[0]; v != 6.5 {
		t.Errorf("QMU product-weighted records = %v, want 6.5", v)
	}

	// Order independence: urgency x impact must equal impact x urgency.
	g2, err := m.EvaluateScaled("samp.yaml", cfg, urgency, impact)
	if err != nil {
		t.Fatal(err)
	}
	if g2.Cells[0].Values[0] != 6.5 {
		t.Errorf("scaling order changed the result: %v", g2.Cells[0].Values[0])
	}
}

// TestEvaluateScaled_WeightsNumericSum is the per-app heaviness case: weight a
// numeric measure (a range/score field) by a categorical factor, so each
// record contributes factor*value. sum(score) scale "urgency" per app gives
// architectuur_factor x fcdm_dekking.
func TestEvaluateScaled_WeightsNumericSum(t *testing.T) {
	scored := func(file, qzm string, score float64, app string) index.FormRow {
		s := score
		return index.FormRow{
			Template: "samp.yaml", Filename: file, Mtime: 1,
			Values: []index.FormValueRow{
				{FieldKey: "app", ValueType: "text", Text: app},
				{FieldKey: "score", ValueType: "number", Num: &s},
			},
			Facets: []index.FormFacet{{Key: "qzm", Set: true, Selected: qzm}},
		}
	}
	forms := []index.FormRow{
		scored("a.meta.json", "NIET ZONNIG", 100, "Alpha"), // factor 2 * 100 = 200
		scored("b.meta.json", "ZONNIG", 80, "Beta"),        // factor 0.5 * 80 = 40
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	cfg, err := Parse(`sum(F["score"]) by F["app"]`)
	if err != nil {
		t.Fatal(err)
	}
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 0.5}, {Label: "NIET ZONNIG", Factor: 2}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("samp.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, c := range g.Cells {
		got[g.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	if got["Alpha"] != 200 {
		t.Errorf("Alpha weighted sum = %v, want 200 (2*100)", got["Alpha"])
	}
	if got["Beta"] != 40 {
		t.Errorf("Beta weighted sum = %v, want 40 (0.5*80)", got["Beta"])
	}

	// Without scaling the same measure is the raw value, proving the factor is
	// what changes it.
	plain, err := m.Evaluate("samp.yaml", cfg)
	if err != nil {
		t.Fatal(err)
	}
	pgot := map[string]float64{}
	for _, c := range plain.Cells {
		pgot[plain.Axes[0].Labels[c.Coords[0]]] = c.Values[0]
	}
	if pgot["Alpha"] != 100 || pgot["Beta"] != 80 {
		t.Errorf("unscaled sums Alpha=%v Beta=%v, want 100 and 80", pgot["Alpha"], pgot["Beta"])
	}
}

// TestEvaluateScaled_CountWeightedPerRow checks count() scaling sums a factor
// per row (mentions), not per distinct form.
func TestEvaluateScaled_CountWeightedPerRow(t *testing.T) {
	forms := []index.FormRow{
		qzmForm("x.meta.json", "ZONNIG", "QMU", "QMU"), // 2 rows x 0.5
		qzmForm("y.meta.json", "NIET ZONNIG", "QMU"),   // 1 row x 2
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	cfg, _ := Parse(`count() by F["components"]["item"] top 10`)
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 0.5}, {Label: "NIET ZONNIG", Factor: 2}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("samp.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	// QMU: 0.5 + 0.5 + 2 = 3
	if g.Cells[0].Values[0] != 3 {
		t.Errorf("QMU weighted count = %v, want 3", g.Cells[0].Values[0])
	}
}

// TestService_EvaluateObject_ResolvesScaleByName is the unified+reusable path:
// a plain object references a separate scaling object by name through its DSL
// scale clause, and the Service resolves it before evaluating.
func TestService_EvaluateObject_ResolvesScaleByName(t *testing.T) {
	forms := []index.FormRow{
		qzmForm("x.meta.json", "ZONNIG", "QMU"),      // 0.5
		qzmForm("y.meta.json", "NIET ZONNIG", "QMU"), // 2
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	catalog := []StatObject{
		{
			Name: "gas-apps",
			DSL:  `records() by F["components"]["item"] top 10 scale "qzm-urgency"`,
		},
		{
			Name: "qzm-urgency",
			Scaling: &Scaling{
				Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
				Weights: []WeightEntry{{Label: "ZONNIG", Factor: 0.5}, {Label: "NIET ZONNIG", Factor: 2}},
				Default: 1,
			},
		},
	}
	svc := NewService(m, fakeSource{list: catalog})

	g, err := svc.EvaluateObject("samp.yaml", "gas-apps")
	if err != nil {
		t.Fatal(err)
	}
	// QMU: 0.5 (x) + 2 (y) = 2.5
	if g.Cells[0].Values[0] != 2.5 {
		t.Errorf("QMU weighted records = %v, want 2.5", g.Cells[0].Values[0])
	}

	// Evaluating the scaling object directly is an error (no grid of its own).
	if _, err := svc.EvaluateObject("samp.yaml", "qzm-urgency"); err == nil {
		t.Error("expected error evaluating a scaling object directly")
	}
	// An unknown scale name surfaces as an error, not a silent unweighted run.
	bad := []StatObject{{Name: "g", DSL: `records() by F["components"]["item"] scale "ghost"`}}
	svc2 := NewService(m, fakeSource{list: bad})
	if _, err := svc2.EvaluateObject("samp.yaml", "g"); err == nil {
		t.Error("expected error for unknown scale name")
	}
}

// TestEvaluateScaled_PctFormsUsesWeightedDenominator is the regression for the
// apples-to-pears percentage bug: with scaling, the cell values are weighted
// sums, so `pct forms` must divide by the weighted form total, not the raw
// form count (which produced >100% nonsense like 153%).
func TestEvaluateScaled_PctFormsUsesWeightedDenominator(t *testing.T) {
	forms := []index.FormRow{
		qzmForm("r1.meta.json", "NIET ZONNIG", "QMU"),  // factor 4
		qzmForm("r2.meta.json", "NIET ZONNIG", "QMU"),  // factor 4
		qzmForm("r3.meta.json", "ZONNIG", "Bladework"), // factor 1
		qzmForm("r4.meta.json", "ZONNIG", "Bladework"), // factor 1
	}
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"components.item": 0}})

	cfg, _ := Parse(`records() by F["components"]["item"] pct forms`)
	sc := &Scaling{
		Source:  SourceRef{Kind: SourceFacet, Key: "qzm"},
		Weights: []WeightEntry{{Label: "ZONNIG", Factor: 1}, {Label: "NIET ZONNIG", Factor: 4}},
		Default: 1,
	}
	g, err := m.EvaluateScaled("samp.yaml", cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	// Weighted form total = 4+4+1+1 = 10 (not the raw count 4). QMU weighs 8,
	// so its forms share is 8/10 = 80%, not the broken 8/4 = 200%.
	pct := map[string]float64{}
	val := map[string]float64{}
	for _, c := range g.Cells {
		label := g.Axes[0].Labels[c.Coords[0]]
		val[label] = c.Values[0]
		pct[label] = c.Pct[0]
	}
	if val["QMU"] != 8 {
		t.Errorf("QMU weighted records = %v, want 8", val["QMU"])
	}
	if !nearly(pct["QMU"], 80) {
		t.Errorf("QMU pct forms = %v, want 80 (weighted denominator 10)", pct["QMU"])
	}
	if !nearly(pct["Bladework"], 20) {
		t.Errorf("Bladework pct forms = %v, want 20", pct["Bladework"])
	}
}

// TestEvaluateScaled_RejectsTableColumnSource guards the per-form rule: a
// table-column scaling source has no single per-form weight.
func TestEvaluateScaled_RejectsTableColumnSource(t *testing.T) {
	m := NewManager(datacoreBackend(nil))
	cfg, _ := Parse(`records() by F["base-table"]`)
	sc := &Scaling{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}, Default: 1}
	if _, err := m.EvaluateScaled("samp.yaml", cfg, sc); err == nil {
		t.Fatal("expected error for table-column scaling source")
	}
}
