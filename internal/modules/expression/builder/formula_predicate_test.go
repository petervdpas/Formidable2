package builder

import (
	"strings"
	"testing"
)

// formulasThree: a number, a bool and a date formula, plus a text one
// that carries no rule kind, so tests can assert both the accepted and
// the rejected halves of the catalog.
func formulasThree() []FormulaRef {
	return []FormulaRef{
		{Key: "margin", Type: "number"},
		{Key: "at-risk", Type: "bool"},
		{Key: "deadline", Type: "date"},
		{Key: "summary", Type: "text"},
	}
}

// ── KindForFormula ───────────────────────────────────────────────

func TestKindForFormula(t *testing.T) {
	cases := []struct {
		in   string
		want RuleKind
		ok   bool
	}{
		{"number", KindNumber, true},
		{"NUMBER", KindNumber, true},
		{"  number  ", KindNumber, true},
		{"", KindNumber, true},
		{"   ", KindNumber, true},
		{"bool", KindBoolean, true},
		{"date", KindDate, true},
		{"text", "", false},
		{"boolean", "", false},
		{"unknown", "", false},
	}
	for _, c := range cases {
		got, ok := KindForFormula(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("KindForFormula(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// A blank formula type means "number" in exactly one place; if the
// template package's default ever moves, this fails rather than
// silently dropping untyped formulas out of the predicate picker.
func TestKindForFormula_BlankMatchesTemplateDefault(t *testing.T) {
	blank, _ := KindForFormula("")
	explicit, _ := KindForFormula("number")
	if blank != explicit {
		t.Errorf("blank formula type resolved to %q, want the same kind as number (%q)", blank, explicit)
	}
}

// ── DefaultPredicateForFormula ───────────────────────────────────

func TestDefaultPredicateForFormula_Number(t *testing.T) {
	p, err := DefaultPredicateForFormula("number", "margin")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Kind != KindNumber || p.FieldKey != "margin" || p.NumberOp != NumberOpEq {
		t.Errorf("shape: %+v", p)
	}
	if p.NumberValue == nil || *p.NumberValue != 0 {
		t.Errorf("NumberValue should default to 0; got %v", p.NumberValue)
	}
}

func TestDefaultPredicateForFormula_Bool(t *testing.T) {
	p, err := DefaultPredicateForFormula("bool", "at-risk")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Kind != KindBoolean || p.FieldKey != "at-risk" {
		t.Errorf("shape: %+v", p)
	}
	if p.BoolValue == nil || !*p.BoolValue {
		t.Errorf("BoolValue should default to true; got %v", p.BoolValue)
	}
}

func TestDefaultPredicateForFormula_Date(t *testing.T) {
	p, err := DefaultPredicateForFormula("date", "deadline")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Kind != KindDate || p.DateOp != DateOpIsOverdue {
		t.Errorf("shape: %+v", p)
	}
	if p.DateArg != nil {
		t.Errorf("DateArg should default to unset; got %v", p.DateArg)
	}
}

func TestDefaultPredicateForFormula_BlankTypeIsNumber(t *testing.T) {
	p, err := DefaultPredicateForFormula("", "untyped")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Kind != KindNumber {
		t.Errorf("blank formula type should default to number; got %q", p.Kind)
	}
}

func TestDefaultPredicateForFormula_TextIsError(t *testing.T) {
	if _, err := DefaultPredicateForFormula("text", "summary"); err == nil {
		t.Error("expected error: text formulas carry no rule kind")
	}
}

func TestDefaultPredicateForFormula_EmptyKeyIsError(t *testing.T) {
	if _, err := DefaultPredicateForFormula("number", ""); err == nil {
		t.Error("expected error for empty key")
	}
	if _, err := DefaultPredicateForFormula("number", "   "); err == nil {
		t.Error("expected error for whitespace-only key")
	}
}

// ── Compile against the formula catalog ──────────────────────────

func TestCompile_NumberFormulaPredicate(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predNumber("margin", NumberOpGt, 10)},
			Outcome:    Outcome{Text: textLiteral("rich")},
		}},
	}
	got, err := Compile(cfg, fieldsFour(), formulasThree())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `F["margin"] > 10 ? {text: L["rich"]} : {}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestCompile_BoolFormulaPredicate(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("at-risk", false)},
			Outcome:    Outcome{Text: textLiteral("safe")},
		}},
	}
	got, err := Compile(cfg, fieldsFour(), formulasThree())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `!F["at-risk"] ? {text: L["safe"]} : {}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestCompile_DateFormulaPredicate(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDateArg("deadline", DateOpIsDueSoon, 7)},
			Outcome:    Outcome{Text: textLiteral("soon")},
		}},
	}
	got, err := Compile(cfg, fieldsFour(), formulasThree())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `isDueSoon(F["deadline"], 7) ? {text: L["soon"]} : {}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestCompile_TextFormulaPredicateIsError(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predEnum("summary", EnumOpEquals, "x")},
			Outcome:    Outcome{Text: textLiteral("x")},
		}},
	}
	_, err := Compile(cfg, fieldsFour(), formulasThree())
	if err == nil {
		t.Fatal("expected error: text formulas support no predicates")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Errorf("error should name the formula; got %v", err)
	}
}

func TestCompile_UnknownKeyIsErrorWithFormulasPresent(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predNumber("nope", NumberOpGt, 1)},
			Outcome:    Outcome{Text: textLiteral("x")},
		}},
	}
	_, err := Compile(cfg, fieldsFour(), formulasThree())
	if err == nil {
		t.Fatal("expected error for a key that is neither field nor formula")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should name the key; got %v", err)
	}
}

func TestCompile_FormulaKindMismatchIsError(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID: "r1",
			// margin is a number formula; a boolean predicate on it is a lie.
			Predicates: []Predicate{predBool("margin", true)},
			Outcome:    Outcome{Text: textLiteral("x")},
		}},
	}
	_, err := Compile(cfg, fieldsFour(), formulasThree())
	if err == nil {
		t.Fatal("expected kind-mismatch error")
	}
	if !strings.Contains(err.Error(), "margin") {
		t.Errorf("error should name the formula; got %v", err)
	}
}

// A formula key may not collide with a field key (template validation
// rejects it), but Compile still has to be deterministic if one slips
// through: the real field wins.
func TestCompile_FieldWinsOverCollidingFormulaKey(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", true)},
			Outcome:    Outcome{Text: textLiteral("x")},
		}},
	}
	// "check" is a boolean field in fieldsFour and a number formula here.
	formulas := append(formulasThree(), FormulaRef{Key: "check", Type: "number"})
	got, err := Compile(cfg, fieldsFour(), formulas)
	if err != nil {
		t.Fatalf("field should win over the colliding formula; got %v", err)
	}
	want := `F["check"] ? {text: L["x"]} : {}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestCompile_NoFormulasStillCompilesFieldPredicates(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", true)},
			Outcome:    Outcome{Text: textLiteral("x")},
		}},
	}
	got, err := Compile(cfg, fieldsFour(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `F["check"] ? {text: L["x"]} : {}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// ── Interactions: formulas mixed with fields ─────────────────────

func TestCompile_MixedFieldAndFormulaPredicatesAndOutcome(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID: "r1",
			Predicates: []Predicate{
				predEnum("size", EnumOpEquals, "L"),
				predNumber("margin", NumberOpGe, 5),
				predDate("deadline", DateOpIsOverdue),
			},
			Outcome: Outcome{
				Parts: []TextSource{
					{Kind: TextKindFieldLabel, FieldKey: "size"},
					{Kind: TextKindLiteral, Value: " @ "},
					{Kind: TextKindFieldValue, FieldKey: "margin"},
				},
				Color: "#fff",
			},
		}},
	}
	got, err := Compile(cfg, fieldsFour(), formulasThree())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `(F["size"] == "L" && F["margin"] >= 5 && isOverdue(F["deadline"])) ? ` +
		`{text: str(O["size"]) + str(L[" @ "]) + str(F["margin"]), color: "#fff"} : {}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// ── Round-trip identity ──────────────────────────────────────────

func TestRoundTrip_FormulaPredicates(t *testing.T) {
	fields := fieldsFour()
	formulas := formulasThree()

	roundTrip(t, "number formula", Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predNumber("margin", NumberOpLt, -2.5)},
			Outcome:    Outcome{Text: textLiteral("thin"), Color: "#c00"},
		}},
		Default: Outcome{Text: textLiteral("ok")},
	}, fields, formulas)

	roundTrip(t, "bool formula true", Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("at-risk", true)},
			Outcome:    Outcome{Text: textLiteral("risk")},
		}},
	}, fields, formulas)

	roundTrip(t, "bool formula false", Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("at-risk", false)},
			Outcome:    Outcome{Text: textLiteral("clear")},
		}},
	}, fields, formulas)

	roundTrip(t, "date formula no arg", Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDate("deadline", DateOpIsToday)},
			Outcome:    Outcome{Text: textLiteral("today")},
		}},
	}, fields, formulas)

	roundTrip(t, "date formula with arg", Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDateArg("deadline", DateOpIsOverdueInDays, 3)},
			Outcome:    Outcome{Text: textLiteral("late")},
		}},
	}, fields, formulas)

	roundTrip(t, "ageInDays on a date formula", Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDateArg("deadline", DateOpDateGt, 30)},
			Outcome:    Outcome{Text: textLiteral("stale")},
		}},
	}, fields, formulas)

	roundTrip(t, "field and formula in one rule", Config{
		Rules: []Rule{
			{
				ID: "r1",
				Predicates: []Predicate{
					predBool("check", true),
					predNumber("margin", NumberOpGt, 0),
				},
				Outcome: Outcome{Text: textLiteral("both")},
			},
			{
				ID:         "r2",
				Predicates: []Predicate{predDate("due", DateOpIsFuture)},
				Outcome:    Outcome{Text: textLiteral("later")},
			},
		},
		Default: Outcome{Text: textLiteral("none")},
	}, fields, formulas)

	roundTrip(t, "formula in predicate and outcome", Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predNumber("margin", NumberOpNe, 0)},
			Outcome: Outcome{Parts: []TextSource{
				{Kind: TextKindLiteral, Value: "margin: "},
				{Kind: TextKindFieldValue, FieldKey: "margin"},
			}},
		}},
	}, fields, formulas)
}
