package builder

import (
	"strings"
	"testing"

	"github.com/expr-lang/expr/parser"
)

// ── Helpers ──────────────────────────────────────────────────────

func boolPtr(b bool) *bool      { return &b }
func numPtr(f float64) *float64 { return &f }
func intPtr(i int) *int         { return &i }

func predBool(field string, v bool) Predicate {
	return Predicate{Kind: KindBoolean, FieldKey: field, BoolValue: boolPtr(v)}
}
func predEnum(field string, op EnumOp, vs ...string) Predicate {
	return Predicate{Kind: KindEnum, FieldKey: field, EnumOp: op, EnumValues: vs}
}
func predNumber(field string, op NumberOp, v float64) Predicate {
	return Predicate{Kind: KindNumber, FieldKey: field, NumberOp: op, NumberValue: numPtr(v)}
}
func predDate(field string, op DateOp) Predicate {
	return Predicate{Kind: KindDate, FieldKey: field, DateOp: op}
}
func predDateArg(field string, op DateOp, arg int) Predicate {
	return Predicate{Kind: KindDate, FieldKey: field, DateOp: op, DateArg: intPtr(arg)}
}

func textLiteral(s string) *TextSource { return &TextSource{Kind: TextKindLiteral, Value: s} }
func textValue(field string) *TextSource {
	return &TextSource{Kind: TextKindFieldValue, FieldKey: field}
}
func textLabel(field string) *TextSource {
	return &TextSource{Kind: TextKindFieldLabel, FieldKey: field}
}

// fieldsFour: a typical template — boolean check, dropdown size,
// date due, text title — used as the FieldRef slice for compile
// tests that don't need bespoke option lists.
func fieldsFour() []FieldRef {
	return []FieldRef{
		{Key: "check", Type: "boolean"},
		{Key: "size", Type: "dropdown", Options: []FieldOption{
			{Value: "S", Label: "Small"},
			{Value: "L", Label: "Large"},
		}},
		{Key: "due", Type: "date"},
		{Key: "title", Type: "text"},
		{Key: "score", Type: "number"},
	}
}

// ── Empty-config short-circuit ───────────────────────────────────

func TestCompile_EmptyConfigIsEmptyString(t *testing.T) {
	got, err := Compile(DefaultConfig(), fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "" {
		t.Errorf("empty config should compile to \"\"; got %q", got)
	}
}

func TestCompile_DefaultOutcomeOnlyEmitsBareLiteral(t *testing.T) {
	cfg := Config{Default: Outcome{Color: "gray", Text: textValue("title")}}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `{text: title, color: "gray"}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// ── Predicate kinds ─────────────────────────────────────────────

func TestCompile_BooleanPredicateTrue(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", true)},
			Outcome:    Outcome{Text: textValue("title"), Color: "green"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `check ? {text: title, color: "green"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_BooleanPredicateFalse(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", false)},
			Outcome:    Outcome{Classes: []string{"expr-warn"}},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `!check ? {classes: ["expr-warn"]} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_EnumSingleValueEquals(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predEnum("size", EnumOpEquals, "L")},
			Outcome:    Outcome{Color: "green"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `size == "L" ? {color: "green"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_EnumMultiValueEqualsIsSwitchCase(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predEnum("size", EnumOpEquals, "L", "XL")},
			Outcome:    Outcome{Classes: []string{"expr-bold"}},
		}},
	}
	got, err := Compile(cfg, []FieldRef{
		{Key: "size", Type: "dropdown", Options: []FieldOption{
			{Value: "S", Label: "Small"}, {Value: "L", Label: "Large"}, {Value: "XL", Label: "Extra"},
		}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `(size == "L" || size == "XL") ? {classes: ["expr-bold"]} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_EnumNotEqualsMultiValue(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predEnum("size", EnumOpNotEquals, "S", "M")},
			Outcome:    Outcome{Color: "red"},
		}},
	}
	got, err := Compile(cfg, []FieldRef{
		{Key: "size", Type: "dropdown", Options: []FieldOption{{Value: "S"}, {Value: "M"}}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `(size != "S" && size != "M") ? {color: "red"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_NumberGreaterThan(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predNumber("score", NumberOpGt, 10)},
			Outcome:    Outcome{Color: "orange"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `score > 10 ? {color: "orange"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_NumberFractionalValue(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predNumber("score", NumberOpLe, 2.5)},
			Outcome:    Outcome{Bg: "#fff"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `score <= 2.5 ? {bg: "#fff"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_DateNoArg(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDate("due", DateOpIsOverdue)},
			Outcome:    Outcome{Color: "red"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `isOverdue(due) ? {color: "red"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_DateGtUsesAgeInDays(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDateArg("due", DateOpDateGt, 30)},
			Outcome:    Outcome{Color: "red"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `ageInDays(due) > 30 ? {color: "red"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_DateLtUsesAgeInDays(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDateArg("due", DateOpDateLt, 7)},
			Outcome:    Outcome{Color: "blue"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `ageInDays(due) < 7 ? {color: "blue"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_DateGtMissingArgIsError(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDate("due", DateOpDateGt)},
		}},
	}
	if _, err := Compile(cfg, fieldsFour()); err == nil {
		t.Error("expected error for dateGt predicate with no arg")
	}
}

func TestCompile_DateWithArg(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDateArg("due", DateOpIsDueSoon, 7)},
			Outcome:    Outcome{Color: "orange"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `isDueSoon(due, 7) ? {color: "orange"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// ── Cross-field predicates (the whole point) ────────────────────

func TestCompile_RulePredicatesAreANDed(t *testing.T) {
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
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `(check && size == "L" && isOverdue(due)) ? {color: "red"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_EmptyPredicatesAlwaysMatches(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{},
			Outcome:    Outcome{Color: "blue"},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `true ? {color: "blue"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// ── TextSource ──────────────────────────────────────────────────

func TestCompile_TextLiteral(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", true)},
			Outcome:    Outcome{Text: textLiteral("DONE")},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `check ? {text: "DONE"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_TextFieldValueIsBareReference(t *testing.T) {
	cfg := Config{
		Default: Outcome{Text: textValue("title")},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `{text: title}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_TextFieldLabelBakesOptionLookup(t *testing.T) {
	cfg := Config{
		Default: Outcome{Text: textLabel("size")},
	}
	got, err := Compile(cfg, []FieldRef{
		{Key: "size", Type: "dropdown", Options: []FieldOption{
			{Value: "S", Label: "Small"},
			{Value: "L", Label: "Large"},
		}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Outermost ternary unwrapped, inner one wrapped in parens.
	want := `{text: size == "S" ? "Small" : (size == "L" ? "Large" : size)}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_TextFieldLabelOnFieldWithoutOptionsFallsBackToValue(t *testing.T) {
	cfg := Config{
		Default: Outcome{Text: textLabel("title")}, // text field, no options
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `{text: title}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// ── Cascade and the user's worked example ───────────────────────

func TestCompile_MultipleRulesNestedTernary(t *testing.T) {
	cfg := Config{
		Rules: []Rule{
			{ID: "r1", Predicates: []Predicate{predEnum("size", EnumOpEquals, "L")}, Outcome: Outcome{Color: "green"}},
			{ID: "r2", Predicates: []Predicate{predEnum("size", EnumOpEquals, "XL")}, Outcome: Outcome{Color: "red"}},
		},
		Default: Outcome{Color: "gray"},
	}
	got, err := Compile(cfg, []FieldRef{
		{Key: "size", Type: "dropdown", Options: []FieldOption{
			{Value: "L"}, {Value: "XL"},
		}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `size == "L" ? {color: "green"} : (size == "XL" ? {color: "red"} : {color: "gray"})`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_UserWorkedExample(t *testing.T) {
	// From the spec:
	//   if check=false AND dropdown=apple AND date isExpiredAfter(7) → red
	//   if check=true  AND dropdown=pear  AND date isFuture         → green
	//   if dropdown=grapes AND date isToday                          → purple+blink
	//   else                                                         → black scrolling
	// Text always comes from the title field.
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
	got, err := Compile(cfg, []FieldRef{
		{Key: "check", Type: "boolean"},
		{Key: "fruit", Type: "dropdown", Options: []FieldOption{
			{Value: "apple"}, {Value: "pear"}, {Value: "grapes"},
		}},
		{Key: "due", Type: "date"},
		{Key: "title", Type: "text"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `(!check && fruit == "apple" && isExpiredAfter(due, 7)) ? {text: title, color: "red"} : ((check && fruit == "pear" && isFuture(due)) ? {text: title, color: "green"} : ((fruit == "grapes" && isToday(due)) ? {text: title, color: "purple", classes: ["expr-blink"]} : {text: title, color: "black", classes: ["expr-scroll"]}))`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// ── String escaping ─────────────────────────────────────────────

func TestCompile_StyleStringWithDoubleQuotesIsEscaped(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("check", true)},
			Outcome:    Outcome{Text: textLiteral(`He said "hi"`)},
		}},
	}
	got, err := Compile(cfg, fieldsFour())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, `"He said \"hi\""`) {
		t.Errorf("expected JSON-style quote escaping; got %q", got)
	}
}

// ── Unhappy paths ───────────────────────────────────────────────

func TestCompile_PredicateMissingFieldKey(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{{Kind: KindBoolean, BoolValue: boolPtr(true)}},
		}},
	}
	if _, err := Compile(cfg, fieldsFour()); err == nil {
		t.Error("expected error for predicate with empty fieldKey")
	}
}

func TestCompile_PredicateUnknownField(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("ghost", true)},
		}},
	}
	if _, err := Compile(cfg, fieldsFour()); err == nil {
		t.Error("expected error for predicate against unknown field")
	}
}

func TestCompile_PredicateKindMismatchWithFieldType(t *testing.T) {
	// Boolean predicate on a dropdown field is malformed.
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{{Kind: KindBoolean, FieldKey: "size", BoolValue: boolPtr(true)}},
		}},
	}
	if _, err := Compile(cfg, fieldsFour()); err == nil {
		t.Error("expected error for boolean predicate on dropdown field")
	}
}

func TestCompile_BooleanPredicateMissingValue(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{{Kind: KindBoolean, FieldKey: "check"}},
		}},
	}
	if _, err := Compile(cfg, fieldsFour()); err == nil {
		t.Error("expected error for boolean predicate with nil BoolValue")
	}
}

func TestCompile_EnumPredicateEmptyValues(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{{Kind: KindEnum, FieldKey: "size", EnumOp: EnumOpEquals}},
		}},
	}
	if _, err := Compile(cfg, fieldsFour()); err == nil {
		t.Error("expected error for enum predicate with no values")
	}
}

func TestCompile_NumberPredicateMissingValue(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{{Kind: KindNumber, FieldKey: "score", NumberOp: NumberOpGt}},
		}},
	}
	if _, err := Compile(cfg, fieldsFour()); err == nil {
		t.Error("expected error for number predicate with nil NumberValue")
	}
}

func TestCompile_TextSourceUnknownKind(t *testing.T) {
	cfg := Config{
		Default: Outcome{Text: &TextSource{Kind: "garbage"}},
	}
	if _, err := Compile(cfg, fieldsFour()); err == nil {
		t.Error("expected error for unknown text source kind")
	}
}

// ── Hyphenated field keys ───────────────────────────────────────
//
// Template field keys like `unit-number` or `street-address` are
// valid templating identifiers but illegal as bare expr-lang
// identifiers (the lexer reads `unit-number` as `unit - number`).
// Compile must emit the $env["..."] map-lookup form for those
// keys everywhere a field reference appears: predicate sides,
// helper-call args, text sources, and the bakeOptionLookup ternary.

func TestCompile_BooleanPredicateOnHyphenKeyUsesDollarEnv(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("has-license", true)},
			Outcome:    Outcome{Color: "green"},
		}},
	}
	got, err := Compile(cfg, []FieldRef{{Key: "has-license", Type: "boolean"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `$env["has-license"] ? {color: "green"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_BooleanPredicateNegatedHyphenKey(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("has-license", false)},
			Outcome:    Outcome{Color: "red"},
		}},
	}
	got, err := Compile(cfg, []FieldRef{{Key: "has-license", Type: "boolean"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `!$env["has-license"] ? {color: "red"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_EnumEqualsOnHyphenKey(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predEnum("payment-status", EnumOpEquals, "paid")},
			Outcome:    Outcome{Color: "green"},
		}},
	}
	got, err := Compile(cfg, []FieldRef{
		{Key: "payment-status", Type: "dropdown", Options: []FieldOption{{Value: "paid"}, {Value: "due"}}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `$env["payment-status"] == "paid" ? {color: "green"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_NumberOpOnHyphenKey(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predNumber("score-total", NumberOpGt, 10)},
			Outcome:    Outcome{Color: "orange"},
		}},
	}
	got, err := Compile(cfg, []FieldRef{{Key: "score-total", Type: "number"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `$env["score-total"] > 10 ? {color: "orange"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_DateHelperOnHyphenKey(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDate("due-date", DateOpIsOverdue)},
			Outcome:    Outcome{Color: "red"},
		}},
	}
	got, err := Compile(cfg, []FieldRef{{Key: "due-date", Type: "date"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `isOverdue($env["due-date"]) ? {color: "red"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_DateGtOnHyphenKey(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predDateArg("due-date", DateOpDateGt, 30)},
			Outcome:    Outcome{Color: "red"},
		}},
	}
	got, err := Compile(cfg, []FieldRef{{Key: "due-date", Type: "date"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `ageInDays($env["due-date"]) > 30 ? {color: "red"} : {}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_TextFieldValueOnHyphenKey(t *testing.T) {
	cfg := Config{Default: Outcome{Text: textValue("street-address")}}
	got, err := Compile(cfg, []FieldRef{{Key: "street-address", Type: "text"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `{text: $env["street-address"]}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_TextFieldLabelOnHyphenKeyBakesWithDollarEnv(t *testing.T) {
	cfg := Config{Default: Outcome{Text: textLabel("payment-status")}}
	got, err := Compile(cfg, []FieldRef{
		{Key: "payment-status", Type: "dropdown", Options: []FieldOption{
			{Value: "paid", Label: "Paid"},
			{Value: "due", Label: "Outstanding"},
		}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `{text: $env["payment-status"] == "paid" ? "Paid" : ($env["payment-status"] == "due" ? "Outstanding" : $env["payment-status"])}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_PlainKeyStillEmitsBareIdentifier(t *testing.T) {
	cfg := Config{
		Rules: []Rule{{
			ID:         "r1",
			Predicates: []Predicate{predBool("active", true)},
			Outcome:    Outcome{Color: "green"},
		}},
	}
	got, err := Compile(cfg, []FieldRef{{Key: "active", Type: "boolean"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `active ? {color: "green"} : {}`
	if got != want {
		t.Errorf("plain key should stay bare; got %q", got)
	}
}

// ── End-to-end: emitted source parses as valid expr-lang ────────

func TestCompile_EmittedSourceIsValidExprLang(t *testing.T) {
	titleText := textValue("title")
	cases := []struct {
		name string
		cfg  Config
	}{
		{"single boolean rule", Config{
			Rules: []Rule{{ID: "r1", Predicates: []Predicate{predBool("check", true)}, Outcome: Outcome{Color: "green"}}},
		}},
		{"enum multi-value", Config{
			Rules: []Rule{{ID: "r1", Predicates: []Predicate{predEnum("size", EnumOpEquals, "L", "XL")}, Outcome: Outcome{Classes: []string{"expr-bold"}}}},
		}},
		{"cross-field AND", Config{
			Rules: []Rule{{ID: "r1", Predicates: []Predicate{
				predBool("check", true),
				predEnum("size", EnumOpEquals, "L"),
				predDateArg("due", DateOpIsDueSoon, 7),
			}, Outcome: Outcome{Color: "red"}}},
		}},
		{"text label baking", Config{
			Default: Outcome{Text: textLabel("size")},
		}},
		{"user worked example", Config{
			Rules: []Rule{
				{ID: "r1", Predicates: []Predicate{predBool("check", false), predEnum("size", EnumOpEquals, "L"), predDateArg("due", DateOpIsExpiredAfter, 7)}, Outcome: Outcome{Text: titleText, Color: "red"}},
				{ID: "r2", Predicates: []Predicate{predBool("check", true), predDate("due", DateOpIsFuture)}, Outcome: Outcome{Text: titleText, Color: "green"}},
			},
			Default: Outcome{Text: titleText, Color: "black"},
		}},
	}
	fields := []FieldRef{
		{Key: "check", Type: "boolean"},
		{Key: "size", Type: "dropdown", Options: []FieldOption{{Value: "S", Label: "Small"}, {Value: "L", Label: "Large"}, {Value: "XL", Label: "Extra"}}},
		{Key: "due", Type: "date"},
		{Key: "title", Type: "text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, err := Compile(c.cfg, fields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if src == "" {
				t.Fatal("unexpected empty source")
			}
			if _, err := parser.Parse(src); err != nil {
				t.Errorf("emitted source failed parser.Parse: %v\nsrc: %s", err, src)
			}
		})
	}
}
