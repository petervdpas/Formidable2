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
