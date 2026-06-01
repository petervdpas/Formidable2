package template

import "testing"

func TestNormalizeFormulas_TrimsDropsAndDedups(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "id", Type: "guid"}},
		Formulas: []Formula{
			{Key: "  marge ", Label: " M ", Type: "", Expression: " F[\"a\"] * 2 "},
			{Key: "marge", Expression: "F[\"a\"]"}, // duplicate key -> dropped
			{Key: "blank", Expression: "   "},      // empty expression -> dropped
			{Key: "", Expression: "F[\"a\"]"},      // empty key -> dropped
		},
	}
	Normalize(tpl)
	if len(tpl.Formulas) != 1 {
		t.Fatalf("kept %d formulas, want 1: %+v", len(tpl.Formulas), tpl.Formulas)
	}
	f := tpl.Formulas[0]
	if f.Key != "marge" || f.Label != "M" || f.Expression != `F["a"] * 2` {
		t.Errorf("not trimmed: %+v", f)
	}
	if f.Type != "number" {
		t.Errorf("blank type should default to number, got %q", f.Type)
	}
}

func TestFormulasErrors_FlagsStructuralProblems(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "amount", Type: "number"}},
		Facets: []Facet{{Key: "fcdm", Icon: "fa-tag", Options: []FacetOption{{Label: "X", Color: "red"}}}},
		Formulas: []Formula{
			{Key: "Bad Key", Expression: "1"},          // invalid key
			{Key: "amount", Expression: "1"},           // collides with a field
			{Key: "fcdm", Expression: "1"},             // collides with a facet
			{Key: "noexpr", Expression: ""},            // missing expression
			{Key: "wrongtype", Type: "x", Expression: "1"},
			{Key: "dup", Expression: "1"},
			{Key: "dup", Expression: "2"}, // duplicate key
		},
	}
	errs := formulasErrors(tpl)
	for _, want := range []string{
		"invalid-formula-key", "formula-key-collision",
		"formula-missing-expression", "invalid-formula-type", "duplicate-formula-key",
	} {
		if !hasErr(errs, want) {
			t.Errorf("expected a %q error, got %+v", want, errs)
		}
	}
}

func TestFormulasErrors_CleanCatalogPasses(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "amount", Type: "number"}},
		Formulas: []Formula{
			{Key: "marge", Type: "number", Expression: `F["amount"] * 0.21`},
			{Key: "label", Type: "text", Expression: `F["amount"]`},
		},
	}
	if errs := formulasErrors(tpl); len(errs) != 0 {
		t.Errorf("clean catalog should pass, got %+v", errs)
	}
}
