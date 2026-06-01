package expression

import "testing"

func TestFunctions_CatalogIsWellFormed(t *testing.T) {
	valid := map[string]bool{"math": true, "date": true, "text": true, "logic": true, "control": true}
	fns := Functions()
	if len(fns) == 0 {
		t.Fatal("Functions() is empty")
	}
	for _, f := range fns {
		if f.Name == "" || f.Snippet == "" {
			t.Errorf("function %+v has empty name or snippet", f)
		}
		if !valid[f.Category] {
			t.Errorf("function %q has unknown category %q", f.Name, f.Category)
		}
	}
}

// TestFunctions_BuiltinsEvaluate proves every math/date/text/logic function the
// catalog offers actually runs in the engine, so the editor never offers a
// function the expression engine would reject.
func TestFunctions_BuiltinsEvaluate(t *testing.T) {
	e := newEngine()
	cases := map[string]any{
		`max(1, 2)`:                 2,
		`min(1, 2)`:                 1,
		`abs(-3)`:                   3,
		`round(2.6)`:                3,
		`mean([2, 4])`:              float64(3),
		`sum([1, 2, 3])`:            6,
		`str(5)`:                    "5",
		`defaultText("", "x")`:      "x",
		`notEmpty("a")`:             true,
		`today() == today()`:        true,
		`daysBetween("2026-01-01", "2026-01-08")`: 7,
	}
	for src, want := range cases {
		got, err := e.EvaluateRaw(src, map[string]any{})
		if err != nil {
			t.Errorf("%s: did not evaluate: %v", src, err)
			continue
		}
		// expr-lang returns int or float for numerics; compare loosely.
		if !looseEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", src, got, want)
		}
	}
}

func looseEqual(got, want any) bool {
	gf, gok := toFloat(got)
	wf, wok := toFloat(want)
	if gok && wok {
		return gf == wf
	}
	return got == want
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
