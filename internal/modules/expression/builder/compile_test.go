package builder

import (
	"strings"
	"testing"

	"github.com/expr-lang/expr/parser"
)

func boolPtr(b bool) *bool          { return &b }
func numPtr(f float64) *float64     { return &f }
func intPtr(i int) *int             { return &i }
func ruleBool(id string, v bool) Rule {
	return Rule{ID: id, Kind: KindBoolean, BoolValue: boolPtr(v)}
}
func ruleEnum(id string, op EnumOp, vs ...string) Rule {
	return Rule{ID: id, Kind: KindEnum, EnumOp: op, EnumValues: vs}
}
func ruleNumber(id string, op NumberOp, v float64) Rule {
	return Rule{ID: id, Kind: KindNumber, NumberOp: op, NumberValue: numPtr(v)}
}
func ruleDate(id string, op DateOp) Rule {
	return Rule{ID: id, Kind: KindDate, DateOp: op}
}
func ruleDateArg(id string, op DateOp, arg int) Rule {
	return Rule{ID: id, Kind: KindDate, DateOp: op, DateArg: intPtr(arg)}
}

func TestCompile_DisplayOff(t *testing.T) {
	cfg := DefaultFieldConfig() // display: false
	got, err := Compile(cfg, "check")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "" {
		t.Errorf("display off should compile to empty string; got %q", got)
	}
}

func TestCompile_NoRulesEmptyDefault(t *testing.T) {
	cfg := DefaultFieldConfig()
	cfg.Display = true
	got, err := Compile(cfg, "check")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "check" {
		t.Errorf("bare field reference expected; got %q", got)
	}
}

func TestCompile_NoRulesWithDefaultOutcome(t *testing.T) {
	cfg := DefaultFieldConfig()
	cfg.Display = true
	cfg.Default = Outcome{Color: "blue"}
	got, err := Compile(cfg, "name")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != `{text: name, color: "blue"}` {
		t.Errorf("default outcome map literal mismatch; got %q", got)
	}
}

func TestCompile_BooleanTrue(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleBool("r1", true)},
		Styling: map[string]Outcome{"r1": {Color: "green"}},
	}
	got, err := Compile(cfg, "check")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `check ? {text: check, color: "green"} : check`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_BooleanFalse(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleBool("r1", false)},
		Styling: map[string]Outcome{"r1": {Classes: []string{"expr-warn"}}},
	}
	got, err := Compile(cfg, "check")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `!check ? {text: check, classes: ["expr-warn"]} : check`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_EnumSingleValueEquals(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleEnum("r1", EnumOpEquals, "L")},
		Styling: map[string]Outcome{"r1": {Color: "green"}},
	}
	got, err := Compile(cfg, "size")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `size == "L" ? {text: size, color: "green"} : size`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_EnumMultiValueEquals(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleEnum("r1", EnumOpEquals, "L", "XL")},
		Styling: map[string]Outcome{"r1": {Classes: []string{"expr-bold"}}},
	}
	got, err := Compile(cfg, "size")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `(size == "L" || size == "XL") ? {text: size, classes: ["expr-bold"]} : size`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_EnumNotEqualsMultiValue(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleEnum("r1", EnumOpNotEquals, "S", "M")},
		Styling: map[string]Outcome{"r1": {Color: "red"}},
	}
	got, err := Compile(cfg, "size")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `(size != "S" && size != "M") ? {text: size, color: "red"} : size`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_NumberGreaterThan(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleNumber("r1", NumberOpGt, 10)},
		Styling: map[string]Outcome{"r1": {Color: "orange"}},
	}
	got, err := Compile(cfg, "count")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `count > 10 ? {text: count, color: "orange"} : count`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_NumberFractionalValue(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleNumber("r1", NumberOpLe, 2.5)},
		Styling: map[string]Outcome{"r1": {Bg: "#fff"}},
	}
	got, err := Compile(cfg, "score")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `score <= 2.5 ? {text: score, bg: "#fff"} : score`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_DateNoArg(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleDate("r1", DateOpIsOverdue)},
		Styling: map[string]Outcome{"r1": {Color: "red"}},
	}
	got, err := Compile(cfg, "due")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `isOverdue(due) ? {text: due, color: "red"} : due`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_DateWithArg(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleDateArg("r1", DateOpIsDueSoon, 7)},
		Styling: map[string]Outcome{"r1": {Color: "orange"}},
	}
	got, err := Compile(cfg, "due")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `isDueSoon(due, 7) ? {text: due, color: "orange"} : due`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_MultipleRulesNestedTernary(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules: []Rule{
			ruleEnum("r1", EnumOpEquals, "L"),
			ruleEnum("r2", EnumOpEquals, "XL"),
		},
		Styling: map[string]Outcome{
			"r1": {Color: "green"},
			"r2": {Color: "red"},
		},
	}
	got, err := Compile(cfg, "size")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// First rule wins; later rule's predicate is the inner else.
	want := `size == "L" ? {text: size, color: "green"} : (size == "XL" ? {text: size, color: "red"} : size)`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_RulesCascadeToCustomDefault(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleBool("r1", true)},
		Styling: map[string]Outcome{"r1": {Color: "green"}},
		Default: Outcome{Color: "gray"},
	}
	got, err := Compile(cfg, "check")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `check ? {text: check, color: "green"} : {text: check, color: "gray"}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_TextOverrideInOutcome(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleBool("r1", true)},
		Styling: map[string]Outcome{"r1": {Text: "YES", Color: "green"}},
	}
	got, err := Compile(cfg, "check")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `check ? {text: "YES", color: "green"} : check`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCompile_StyleStringWithDoubleQuotesIsEscaped(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{ruleBool("r1", true)},
		Styling: map[string]Outcome{"r1": {Text: `He said "hi"`}},
	}
	got, err := Compile(cfg, "check")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, `"He said \"hi\""`) {
		t.Errorf("expected JSON-style quote escaping; got %q", got)
	}
}

// ── Unhappy paths ────────────────────────────────────────────────

func TestCompile_BooleanRuleMissingValue(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{{ID: "r1", Kind: KindBoolean}},
	}
	if _, err := Compile(cfg, "check"); err == nil {
		t.Error("expected error for boolean rule with nil BoolValue")
	}
}

func TestCompile_NumberRuleMissingValue(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{{ID: "r1", Kind: KindNumber, NumberOp: NumberOpGt}},
	}
	if _, err := Compile(cfg, "count"); err == nil {
		t.Error("expected error for number rule with nil NumberValue")
	}
}

func TestCompile_EnumRuleEmptyValues(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{{ID: "r1", Kind: KindEnum, EnumOp: EnumOpEquals}},
	}
	if _, err := Compile(cfg, "size"); err == nil {
		t.Error("expected error for enum rule with no values")
	}
}

func TestCompile_EnumRuleUnknownOp(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{{ID: "r1", Kind: KindEnum, EnumOp: "garbage", EnumValues: []string{"A"}}},
	}
	if _, err := Compile(cfg, "size"); err == nil {
		t.Error("expected error for enum rule with unknown op")
	}
}

func TestCompile_UnknownRuleKind(t *testing.T) {
	cfg := FieldConfig{
		Display: true,
		Rules:   []Rule{{ID: "r1", Kind: "garbage"}},
	}
	if _, err := Compile(cfg, "x"); err == nil {
		t.Error("expected error for unknown rule kind")
	}
}

func TestCompile_EmptyFieldKey(t *testing.T) {
	cfg := DefaultFieldConfig()
	cfg.Display = true
	if _, err := Compile(cfg, ""); err == nil {
		t.Error("expected error for empty field key")
	}
	if _, err := Compile(cfg, "   "); err == nil {
		t.Error("expected error for whitespace-only field key")
	}
}

// ── End-to-end: emitted source parses as valid expr-lang ────────
//
// Belt-and-braces: every well-typed config compiles to syntactically
// valid expr-lang. We use parser.Parse (not expr.Compile) because
// type-checking would collide with built-ins that share user field
// names — e.g. a field named `count` shadows expr's `count` filter
// helper at runtime via the env, but the compile-time type-checker
// can't see that. Syntax validity is what Compile is responsible
// for; the engine's runtime env handles identifier resolution.

func TestCompile_EmittedSourceIsValidExprLang(t *testing.T) {
	cases := []struct {
		name string
		cfg  FieldConfig
		key  string
	}{
		{"bare field", FieldConfig{Display: true}, "check"},
		{
			"boolean rule",
			FieldConfig{
				Display: true,
				Rules:   []Rule{ruleBool("r1", true)},
				Styling: map[string]Outcome{"r1": {Color: "green"}},
			},
			"check",
		},
		{
			"enum multi-value",
			FieldConfig{
				Display: true,
				Rules:   []Rule{ruleEnum("r1", EnumOpEquals, "L", "XL")},
				Styling: map[string]Outcome{"r1": {Classes: []string{"expr-bold"}}},
			},
			"size",
		},
		{
			"number cascade with default",
			FieldConfig{
				Display: true,
				Rules: []Rule{
					ruleNumber("r1", NumberOpGt, 10),
					ruleNumber("r2", NumberOpLt, 0),
				},
				Styling: map[string]Outcome{
					"r1": {Color: "red"},
					"r2": {Color: "blue"},
				},
				Default: Outcome{Color: "gray"},
			},
			"count",
		},
		{
			"date with arg",
			FieldConfig{
				Display: true,
				Rules:   []Rule{ruleDateArg("r1", DateOpIsDueSoon, 7)},
				Styling: map[string]Outcome{"r1": {Color: "orange"}},
			},
			"due",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, err := Compile(c.cfg, c.key)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if _, err := parser.Parse(src); err != nil {
				t.Errorf("emitted source failed parser.Parse: %v\nsrc: %s", err, src)
			}
		})
	}
}
