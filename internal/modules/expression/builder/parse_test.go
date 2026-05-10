package builder

import (
	"strings"
	"testing"
)

// roundTrip is the contract: Compile(cfg) → Parse(src) → Compile(cfg2)
// must equal the original source string. We don't compare Config
// shapes directly because rule IDs are session-scoped (Compile
// doesn't read them; Parse re-assigns r1/r2/...) — instead we close
// the loop with a second Compile and compare strings.
func roundTrip(t *testing.T, name string, cfg Config, fields []FieldRef) {
	t.Helper()
	src, err := Compile(cfg, fields)
	if err != nil {
		t.Fatalf("[%s] compile #1: %v", name, err)
	}

	parsed, err := Parse(src, fields)
	if err != nil {
		t.Fatalf("[%s] parse: %v\nsrc: %s", name, err, src)
	}

	round, err := Compile(parsed, fields)
	if err != nil {
		t.Fatalf("[%s] compile #2: %v", name, err)
	}
	if round != src {
		t.Errorf("[%s] round-trip mismatch\norig:  %s\nround: %s", name, src, round)
	}
}

func TestParse_EmptyString(t *testing.T) {
	cfg, err := Parse("", fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(cfg.Rules) != 0 || cfg.Default.Text != nil {
		t.Errorf("empty input should yield empty config; got %+v", cfg)
	}
}

func TestParse_DefaultOnlyOutcome(t *testing.T) {
	cfg := Config{Default: Outcome{Color: "gray", Text: textValue("title")}}
	roundTrip(t, "default-only", cfg, fieldsFour())
}

// ── Predicate kinds ─────────────────────────────────────────────

func TestParse_BooleanTrue(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", true)},
			Outcome:    Outcome{Text: textValue("title"), Color: "green"},
		}},
	}
	roundTrip(t, "boolean-true", cfg, fieldsFour())
}

func TestParse_BooleanFalse(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", false)},
			Outcome:    Outcome{Classes: []string{"expr-warn"}},
		}},
	}
	roundTrip(t, "boolean-false", cfg, fieldsFour())
}

func TestParse_EnumSingleEquals(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predEnum("size", EnumOpEquals, "L")},
			Outcome:    Outcome{Color: "green"},
		}},
	}
	roundTrip(t, "enum-single-equals", cfg, fieldsFour())
}

func TestParse_EnumMultiEquals(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predEnum("size", EnumOpEquals, "L", "XL")},
			Outcome:    Outcome{Classes: []string{"expr-bold"}},
		}},
	}
	roundTrip(t, "enum-multi-equals", cfg, []FieldRef{
		{Key: "size", Type: "dropdown", Options: []FieldOption{
			{Value: "S"}, {Value: "L"}, {Value: "XL"},
		}},
	})
}

func TestParse_EnumNotEqualsMulti(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predEnum("size", EnumOpNotEquals, "S", "M")},
			Outcome:    Outcome{Color: "red"},
		}},
	}
	roundTrip(t, "enum-not-equals-multi", cfg, []FieldRef{
		{Key: "size", Type: "dropdown", Options: []FieldOption{{Value: "S"}, {Value: "M"}}},
	})
}

func TestParse_NumberOps(t *testing.T) {
	cases := []struct {
		name string
		op   NumberOp
		val  float64
	}{
		{"eq", NumberOpEq, 5},
		{"ne", NumberOpNe, 7},
		{"gt", NumberOpGt, 10},
		{"ge", NumberOpGe, 0},
		{"lt", NumberOpLt, -3},
		{"le", NumberOpLe, 2.5},
	}
	for _, c := range cases {
		cfg := Config{
			Rules: []Rule{{
				ID:         "r1",
				Predicates: []Predicate{predNumber("score", c.op, c.val)},
				Outcome:    Outcome{Color: "orange"},
			}},
		}
		roundTrip(t, "number-"+c.name, cfg, fieldsFour())
	}
}

func TestParse_DateNoArg(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDate("due", DateOpIsOverdue)},
			Outcome:    Outcome{Color: "red"},
		}},
	}
	roundTrip(t, "date-no-arg", cfg, fieldsFour())
}

func TestParse_DateWithArg(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDateArg("due", DateOpIsDueSoon, 7)},
			Outcome:    Outcome{Color: "orange"},
		}},
	}
	roundTrip(t, "date-with-arg", cfg, fieldsFour())
}

func TestParse_DateGtAndLt(t *testing.T) {
	for _, op := range []DateOp{DateOpDateGt, DateOpDateLt} {
		cfg := Config{
			Rules: []Rule{{
				ID:         "r1",
				Predicates: []Predicate{predDateArg("due", op, 30)},
				Outcome:    Outcome{Color: "red"},
			}},
		}
		roundTrip(t, "date-"+string(op), cfg, fieldsFour())
	}
}

// ── Cross-field AND ─────────────────────────────────────────────

func TestParse_CrossFieldAnd(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID: "r1",
			Predicates: []Predicate{
				predBool("check", true),
				predEnum("size", EnumOpEquals, "L"),
				predDate("due", DateOpIsOverdue),
			},
			Outcome: Outcome{Color: "red"},
		}},
	}
	roundTrip(t, "cross-field-and", cfg, fieldsFour())
}

func TestParse_EmptyPredicatesAlwaysMatches(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{},
			Outcome:    Outcome{Color: "blue"},
		}},
	}
	roundTrip(t, "empty-predicates", cfg, fieldsFour())
}

// ── Text source ─────────────────────────────────────────────────

func TestParse_TextLiteral(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", true)},
			Outcome:    Outcome{Text: textLiteral("DONE")},
		}},
	}
	roundTrip(t, "text-literal", cfg, fieldsFour())
}

func TestParse_TextFieldValue(t *testing.T) {
	cfg := Config{Default: Outcome{Text: textValue("title")}}
	roundTrip(t, "text-field-value", cfg, fieldsFour())
}

func TestParse_TextFieldLabel(t *testing.T) {
	cfg := Config{Default: Outcome{Text: textLabel("size")}}
	roundTrip(t, "text-field-label", cfg, []FieldRef{
		{Key: "size", Type: "dropdown", Options: []FieldOption{
			{Value: "S", Label: "Small"},
			{Value: "L", Label: "Large"},
		}},
	})
}

// ── Cascade and the worked example ──────────────────────────────

func TestParse_MultipleRulesCascade(t *testing.T) {
	cfg := Config{
		Rules: []Rule{
			{ID: "r1", Predicates: []Predicate{predEnum("size", EnumOpEquals, "L")}, Outcome: Outcome{Color: "green"}},
			{ID: "r2", Predicates: []Predicate{predEnum("size", EnumOpEquals, "XL")}, Outcome: Outcome{Color: "red"}},
		},
		Default: Outcome{Color: "gray"},
	}
	roundTrip(t, "multi-rule-cascade", cfg, []FieldRef{
		{Key: "size", Type: "dropdown", Options: []FieldOption{
			{Value: "L"}, {Value: "XL"},
		}},
	})
}

func TestParse_UserWorkedExample(t *testing.T) {
	titleText := textValue("title")
	cfg := Config{
		Rules: []Rule{
			{
				ID: "r1",
				Predicates: []Predicate{
					predBool("check", false),
					predEnum("fruit", EnumOpEquals, "apple"),
					predDateArg("due", DateOpIsExpiredAfter, 7),
				},
				Outcome: Outcome{Text: titleText, Color: "red"},
			},
			{
				ID: "r2",
				Predicates: []Predicate{
					predBool("check", true),
					predEnum("fruit", EnumOpEquals, "pear"),
					predDate("due", DateOpIsFuture),
				},
				Outcome: Outcome{Text: titleText, Color: "green"},
			},
			{
				ID: "r3",
				Predicates: []Predicate{
					predEnum("fruit", EnumOpEquals, "grapes"),
					predDate("due", DateOpIsToday),
				},
				Outcome: Outcome{Text: titleText, Color: "purple", Classes: []string{"expr-blink"}},
			},
		},
		Default: Outcome{Text: titleText, Color: "black", Classes: []string{"expr-scroll"}},
	}
	roundTrip(t, "worked-example", cfg, []FieldRef{
		{Key: "check", Type: "boolean"},
		{Key: "fruit", Type: "dropdown", Options: []FieldOption{
			{Value: "apple"}, {Value: "pear"}, {Value: "grapes"},
		}},
		{Key: "due", Type: "date"},
		{Key: "title", Type: "text"},
	})
}

// ── Unhappy paths ───────────────────────────────────────────────

func TestParse_RejectsInvalidExprLang(t *testing.T) {
	if _, err := Parse("@@@ not valid @@@", fieldsFour()); err == nil {
		t.Error("expected parse error for invalid expr-lang")
	}
}

func TestParse_RejectsArbitraryShape(t *testing.T) {
	// Plain bare identifier as the whole expression is not a valid
	// outcome (expects map literal); should error.
	if _, err := Parse("title", fieldsFour()); err == nil {
		t.Error("expected error for bare identifier as outcome")
	}
}

func TestParse_RejectsUnknownDateHelper(t *testing.T) {
	src := `unknownHelper(due) ? {color: "red"} : {}`
	if _, err := Parse(src, fieldsFour()); err == nil {
		t.Error("expected error for unknown date helper")
	}
}

func TestParse_RejectsMixedFieldOrGroup(t *testing.T) {
	src := `(size == "L" || other == "X") ? {color: "red"} : {}`
	_, err := Parse(src, fieldsFour())
	if err == nil || !strings.Contains(err.Error(), "mixed fields") {
		t.Errorf("expected mixed-fields error, got %v", err)
	}
}
