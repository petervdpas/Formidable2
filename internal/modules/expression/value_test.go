package expression

import "testing"

func TestEvaluateRaw_ReturnsTypedValues(t *testing.T) {
	e := newEngine()

	// number: F["x"] patched to $env["x"]; arithmetic yields a float.
	got, err := e.EvaluateRaw(`F["amount"] * 0.21`, map[string]any{"amount": 100.0})
	if err != nil {
		t.Fatalf("number: %v", err)
	}
	if f, ok := got.(float64); !ok || f != 21 {
		t.Errorf("number raw = %#v, want float64 21", got)
	}

	// ternary over a categorical field returns the chosen number.
	got, err = e.EvaluateRaw(`F["arch"] == "HOOG" ? 10 : F["arch"] == "LAAG" ? 1 : 0`,
		map[string]any{"arch": "HOOG"})
	if err != nil {
		t.Fatalf("ternary: %v", err)
	}
	if n, ok := got.(int); !ok || n != 10 {
		t.Errorf("ternary raw = %#v, want int 10", got)
	}

	// bool stays bool.
	got, _ = e.EvaluateRaw(`F["n"] > 5`, map[string]any{"n": 9.0})
	if b, ok := got.(bool); !ok || !b {
		t.Errorf("bool raw = %#v, want true", got)
	}

	// a bad expression surfaces an error rather than a styled fallback.
	if _, err := e.EvaluateRaw(`F["a" +`, map[string]any{}); err == nil {
		t.Error("expected a compile error for malformed expression")
	}
}

// TestEvaluate_TextConcatCoercesNumber is the regression for the sidebar
// "invalid operation: string + float64" error: a text field concatenated with a
// number field. A bare `+` errors; the str()-wrapped form the builder now emits
// joins them as strings.
func TestEvaluate_TextConcatCoercesNumber(t *testing.T) {
	e := newEngine()
	ctx := map[string]any{"code": "CH.02", "num": float64(0)}

	// The bug: bare + over a string and a number is an operator error.
	if _, err := e.Evaluate(`F["code"] + F["num"]`, ctx); err == nil {
		t.Error("expected `string + float64` error for a bare concat")
	}

	// The fix: str() coercion makes it plain string joining.
	got, err := e.Evaluate(`str(F["code"]) + str(F["num"])`, ctx)
	if err != nil {
		t.Fatalf("str-wrapped concat errored: %v", err)
	}
	if got.Text != "CH.020" {
		t.Errorf("text = %q, want %q", got.Text, "CH.020")
	}

	// str(nil) is the empty string, so an unset field drops out cleanly.
	got, _ = e.Evaluate(`str(F["code"]) + str(F["missing"])`, ctx)
	if got.Text != "CH.02" {
		t.Errorf("text with unset part = %q, want %q", got.Text, "CH.02")
	}
}

func TestEvaluateFormulas_ChainsInDeclaredOrderAndSkipsErrors(t *testing.T) {
	m := NewManager(nil, nil)
	ctx := map[string]any{"amount": float64(100), "arch": "HOOG"}
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "marge", Type: "number", Expression: `F["amount"] * 0.5`},
		{Key: "weighted", Type: "number", Expression: `F["marge"] + 1`},    // references earlier
		{Key: "label", Type: "text", Expression: `F["arch"] == "HOOG" ? "high" : "low"`},
		{Key: "bad", Type: "number", Expression: `F["amount" *`},           // malformed -> skipped
	}, ctx)

	if got["marge"] != float64(50) {
		t.Errorf("marge = %#v, want 50", got["marge"])
	}
	if got["weighted"] != float64(51) {
		t.Errorf("weighted = %#v, want 51 (declared-order chaining)", got["weighted"])
	}
	if got["label"] != "high" {
		t.Errorf("label = %#v, want high", got["label"])
	}
	if _, ok := got["bad"]; ok {
		t.Errorf("malformed formula should be skipped, got %#v", got["bad"])
	}
	// ctx is mutated so the values are reachable for a follow-on evaluation.
	if ctx["weighted"] != float64(51) {
		t.Errorf("ctx not seeded with weighted: %#v", ctx["weighted"])
	}
}

func TestManagerEvaluateValue_DelegatesToRaw(t *testing.T) {
	m := NewManager(nil, nil)
	got, err := m.EvaluateValue(`F["a"] + F["b"]`, map[string]any{"a": 2.0, "b": 3.0})
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := got.(float64); !ok || f != 5 {
		t.Errorf("EvaluateValue = %#v, want float64 5", got)
	}
}
