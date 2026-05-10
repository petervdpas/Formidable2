package expression

import (
	"strings"
	"testing"
)

func TestEvaluate_PlainString(t *testing.T) {
	e := newEngine()
	got, err := e.Evaluate(`name + " — " + status`, map[string]any{
		"name":   "Audit Control",
		"status": "open",
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.Text != "Audit Control — open" {
		t.Errorf("text: want %q, got %q", "Audit Control — open", got.Text)
	}
	if got.Color != "" || len(got.Classes) != 0 {
		t.Errorf("plain string should produce text only; got %+v", got)
	}
}

func TestEvaluate_StructResult(t *testing.T) {
	e := newEngine()
	got, err := e.Evaluate(
		`{text: name, color: status == "open" ? "red" : "green"}`,
		map[string]any{"name": "x", "status": "open"},
	)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.Text != "x" {
		t.Errorf("text: want x, got %q", got.Text)
	}
	if got.Color != "red" {
		t.Errorf("color: want red, got %q", got.Color)
	}
}

func TestEvaluate_StructWithClasses(t *testing.T) {
	e := newEngine()
	got, err := e.Evaluate(
		`{text: name, classes: ["expr-bold", "expr-warn"]}`,
		map[string]any{"name": "x"},
	)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(got.Classes) != 2 || got.Classes[0] != "expr-bold" || got.Classes[1] != "expr-warn" {
		t.Errorf("classes: %+v", got.Classes)
	}
}

func TestEvaluate_ListResult(t *testing.T) {
	e := newEngine()
	got, err := e.Evaluate(`tags`, map[string]any{
		"tags": []any{"alpha", "beta", "gamma"},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.Text != "alpha, beta, gamma" {
		t.Errorf("list text: want csv, got %q", got.Text)
	}
	if len(got.Items) != 3 {
		t.Errorf("list items: want 3, got %d", len(got.Items))
	}
}

func TestEvaluate_HelperCalls(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	e := newEngine()
	got, err := e.Evaluate(
		`isOverdue(due) ? "OVERDUE" : "on track"`,
		map[string]any{"due": "2026-05-08"},
	)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.Text != "OVERDUE" {
		t.Errorf("isOverdue: want OVERDUE, got %q", got.Text)
	}
}

func TestEvaluate_DefaultTextFallback(t *testing.T) {
	e := newEngine()
	got, err := e.Evaluate(`defaultText(summary, "no summary")`, map[string]any{
		"summary": "",
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.Text != "no summary" {
		t.Errorf("defaultText: got %q", got.Text)
	}
}

func TestEvaluate_MissingVariable(t *testing.T) {
	e := newEngine()
	// `priority` is not in the harvested context — should evaluate to
	// nil rather than blow up. AllowUndefinedVariables is the contract.
	got, err := e.Evaluate(`priority == nil ? "no priority" : priority`, map[string]any{
		"name": "x",
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.Text != "no priority" {
		t.Errorf("missing var: want fallback, got %q", got.Text)
	}
}

func TestEvaluate_SyntaxError(t *testing.T) {
	e := newEngine()
	_, err := e.Evaluate(`name + +`, map[string]any{"name": "x"})
	if err == nil {
		t.Fatal("syntax error should surface")
	}
}

func TestEvaluate_ProgramCache(t *testing.T) {
	e := newEngine()
	src := `name`
	ctx := map[string]any{"name": "alpha"}

	first, err := e.Evaluate(src, ctx)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.Text != "alpha" {
		t.Fatalf("first text: %q", first.Text)
	}

	// Second call should hit the cache; result identical.
	second, err := e.Evaluate(src, ctx)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Text != "alpha" {
		t.Errorf("cached eval drifted: %q", second.Text)
	}

	// Internal: cache map must contain exactly one entry for the
	// source. We don't need to peek at the program itself.
	count := 0
	e.cache.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Errorf("expected 1 cached program, got %d", count)
	}
}

// Hyphenated field keys — `unit-number`, `street-address` etc. —
// are valid in template field definitions but illegal as bare expr-
// lang identifiers (the lexer reads them as subtraction). The
// builder emits `$env["unit-number"]` for those keys; this test
// pins the contract that expr-lang's $env map lookup resolves them
// against the runtime context map.
func TestEvaluate_HyphenatedKeyViaDollarEnv(t *testing.T) {
	e := newEngine()
	got, err := e.Evaluate(
		`$env["unit-number"] + " " + $env["street-address"]`,
		map[string]any{
			"unit-number":    "3",
			"street-address": "Abbey Road",
		},
	)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.Text != "3 Abbey Road" {
		t.Errorf("hyphenated $env lookup: want %q, got %q", "3 Abbey Road", got.Text)
	}
}

// Bare hyphenated identifier must NOT silently work — the lexer
// reads `unit-number` as `unit - number`. With both sides absent
// from env, expr-lang treats them as nil and the subtraction
// fails. This test pins that we never emit bare hyphens from the
// builder.
func TestEvaluate_BareHyphenIdentifierIsSubtraction(t *testing.T) {
	e := newEngine()
	_, err := e.Evaluate(
		`unit-number`,
		map[string]any{"unit-number": "3"},
	)
	if err == nil {
		t.Fatal("bare hyphen identifier evaluated without error — expected subtraction failure")
	}
}

func TestEvaluate_NoIO(t *testing.T) {
	// expr-lang/expr is an expression VM, not a script runtime —
	// confirm the obvious sandbox properties hold by trying a few
	// shapes that should be rejected at compile or run time.
	e := newEngine()
	cases := []string{
		`os.ReadFile("/etc/passwd")`,    // identifier `os` is not in env
		`exec("rm -rf /")`,              // function `exec` is not registered
		`while (true) { name }`,         // not in expr-lang grammar
	}
	for _, src := range cases {
		_, err := e.Evaluate(src, map[string]any{"name": "x"})
		if err == nil {
			t.Errorf("expected rejection for %q", src)
			continue
		}
		// Sanity: the error mentions either a missing identifier or a
		// parse error — either is fine; just don't silently succeed.
		msg := err.Error()
		if !strings.Contains(msg, "unknown") && !strings.Contains(msg, "expected") &&
			!strings.Contains(msg, "undefined") && !strings.Contains(msg, "unexpected") &&
			!strings.Contains(msg, "cannot") {
			t.Logf("non-fatal: rejection error %q for %q (kept for visibility)", msg, src)
		}
	}
}
