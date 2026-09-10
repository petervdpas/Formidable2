package expression

import "testing"

// A formula's declared type is a promise its consumers rely on: the sidebar
// builder compiles `F["k"] > 10` for a number formula and `F["k"]` as a bare
// ternary condition for a bool one. expr-lang raises on `string > int` and on a
// non-bool condition, so a value that contradicts the declared type has to be
// dropped rather than passed through or zero-filled. Dropping reuses the rule
// EvaluateFormulas already applies to a formula that fails to evaluate.

func TestEvaluateFormulas_NumberKeepsNumericResults(t *testing.T) {
	m := NewManager(nil, nil)
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "f", Type: "number", Expression: `2.5 * 2`},
		{Key: "i", Type: "number", Expression: `3 + 4`},
	}, map[string]any{})

	if got["f"] != float64(5) {
		t.Errorf("f = %#v, want float64(5)", got["f"])
	}
	// Integers normalise to float64 so every number formula has one Go type.
	if got["i"] != float64(7) {
		t.Errorf("i = %#v, want float64(7)", got["i"])
	}
}

func TestEvaluateFormulas_NumberAcceptsNumericString(t *testing.T) {
	m := NewManager(nil, nil)
	ctx := map[string]any{"a": "42"}
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "n", Type: "number", Expression: `F["a"]`},
	}, ctx)

	// The datacore path already round-trips numbers through string cells, so
	// refusing a numeric string here would make the two consumers of the same
	// formula disagree about its value.
	if got["n"] != float64(42) {
		t.Errorf("n = %#v, want float64(42)", got["n"])
	}
	if ctx["n"] != float64(42) {
		t.Errorf("ctx should carry the coerced value, got %#v", ctx["n"])
	}
}

func TestEvaluateFormulas_NumberDropsNonNumericResult(t *testing.T) {
	m := NewManager(nil, nil)
	ctx := map[string]any{"a": "abc"}
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "n", Type: "number", Expression: `F["a"]`},
		{Key: "after", Type: "number", Expression: `1 + 1`},
	}, ctx)

	if v, ok := got["n"]; ok {
		t.Errorf("a non-numeric number formula should be dropped, got %#v", v)
	}
	if _, ok := ctx["n"]; ok {
		t.Errorf("a dropped formula must not seed ctx: %#v", ctx["n"])
	}
	if got["after"] != float64(2) {
		t.Errorf("one dropped formula must not abort the rest; after = %#v", got["after"])
	}
}

func TestEvaluateFormulas_BoolKeepsBoolAndParsesBoolString(t *testing.T) {
	m := NewManager(nil, nil)
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "b", Type: "bool", Expression: `1 < 2`},
		{Key: "s", Type: "bool", Expression: `"true"`},
		{Key: "f", Type: "bool", Expression: `"false"`},
	}, map[string]any{})

	if got["b"] != true {
		t.Errorf("b = %#v, want true", got["b"])
	}
	if got["s"] != true {
		t.Errorf("s = %#v, want true", got["s"])
	}
	if got["f"] != false {
		t.Errorf("f = %#v, want false", got["f"])
	}
}

func TestEvaluateFormulas_BoolDropsNonBooleanResult(t *testing.T) {
	m := NewManager(nil, nil)
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "word", Type: "bool", Expression: `"yes"`},
		{Key: "num", Type: "bool", Expression: `1`},
	}, map[string]any{})

	for _, k := range []string{"word", "num"} {
		if v, ok := got[k]; ok {
			t.Errorf("%s should be dropped, got %#v", k, v)
		}
	}
}

// Date and text stay as-is. Every date helper takes `any` and has a defined
// answer for garbage (helpers.go), so a date formula can never raise; text is
// only ever wrapped in str(). Narrowing either would break that tolerance
// without preventing a single runtime error.
func TestEvaluateFormulas_DateAndTextPassThrough(t *testing.T) {
	m := NewManager(nil, nil)
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "d", Type: "date", Expression: `"not-a-date"`},
		{Key: "n", Type: "date", Expression: `12.5`},
		{Key: "t", Type: "text", Expression: `1 + 1`},
	}, map[string]any{})

	if got["d"] != "not-a-date" {
		t.Errorf("d = %#v, want the raw string", got["d"])
	}
	if got["n"] != float64(12.5) {
		t.Errorf("n = %#v, want the raw number", got["n"])
	}
	// Untouched means untouched: expr-lang returns int for integer arithmetic,
	// and a text formula keeps that Go type rather than being normalised.
	if got["t"] != 2 {
		t.Errorf("t = %#v, want the raw int result", got["t"])
	}
}

func TestEvaluateFormulas_BlankTypeCoercesAsNumber(t *testing.T) {
	m := NewManager(nil, nil)
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "ok", Expression: `"42"`},
		{Key: "bad", Expression: `"abc"`},
	}, map[string]any{})

	if got["ok"] != float64(42) {
		t.Errorf("ok = %#v, want float64(42): a blank type is a number", got["ok"])
	}
	if v, ok := got["bad"]; ok {
		t.Errorf("bad should be dropped like any number formula, got %#v", v)
	}
}

// The coerced value, not the raw one, is what a later formula sees.
func TestEvaluateFormulas_ChainSeesCoercedValue(t *testing.T) {
	m := NewManager(nil, nil)
	got := m.EvaluateFormulas([]FormulaSpec{
		{Key: "base", Type: "number", Expression: `"10"`},
		{Key: "double", Type: "number", Expression: `F["base"] * 2`},
	}, map[string]any{})

	if got["base"] != float64(10) {
		t.Fatalf("base = %#v, want float64(10)", got["base"])
	}
	if got["double"] != float64(20) {
		t.Errorf("double = %#v, want float64(20): the chain must see the coerced value", got["double"])
	}
}

// End to end with the builder's output: a well-typed number formula evaluates,
// and a dropped one surfaces as an evaluation error rather than a wrong chip.
func TestEvaluateFormulas_FeedsSidebarPredicate(t *testing.T) {
	m := NewManager(nil, nil)
	src := `F["margin"] > 10 ? {text: L["rich"]} : {text: L["thin"]}`

	ctx := map[string]any{"amount": "50"}
	m.EvaluateFormulas([]FormulaSpec{
		{Key: "margin", Type: "number", Expression: `F["amount"]`},
	}, ctx)
	res, err := m.Evaluate(src, ctx)
	if err != nil {
		t.Fatalf("a coerced numeric string should compare cleanly: %v", err)
	}
	if res.Text != "rich" {
		t.Errorf("text = %q, want rich", res.Text)
	}

	broken := map[string]any{"amount": "abc"}
	m.EvaluateFormulas([]FormulaSpec{
		{Key: "margin", Type: "number", Expression: `F["amount"]`},
	}, broken)
	if _, err := m.Evaluate(src, broken); err == nil {
		t.Error("a dropped formula must error visibly, not render a confident wrong chip")
	}
}
