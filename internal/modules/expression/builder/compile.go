package builder

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Compile turns a FieldConfig into an expr-lang source string the
// engine can evaluate against a record's harvested ExpressionItems.
//
// Output rules:
//   - display=false → "" (frontend hides the sub-label entirely)
//   - no rules + empty default → bare field reference (engine wraps
//     it as {Text: <value>} via normalize)
//   - no rules + custom default → default outcome map literal
//   - rules → ternary chain (first match wins), inner else is the
//     next rule, terminal else is the default
//
// fieldKey is the variable name that resolves against the record
// context (typically the Field.Key). Empty/whitespace fieldKey is an
// error — the engine would fail anyway, but catching it here gives a
// builder-shaped error message.
func Compile(cfg FieldConfig, fieldKey string) (string, error) {
	if !cfg.Display {
		return "", nil
	}
	if strings.TrimSpace(fieldKey) == "" {
		return "", fmt.Errorf("builder: field key required")
	}

	tail := defaultExpr(cfg.Default, fieldKey)
	if len(cfg.Rules) == 0 {
		return tail, nil
	}

	// Walk rules in reverse so the innermost else is the default and
	// each outer ternary's else is the next rule's predicate.
	for i := len(cfg.Rules) - 1; i >= 0; i-- {
		r := cfg.Rules[i]
		pred, err := predicate(r, fieldKey)
		if err != nil {
			return "", err
		}
		thenExpr := outcomeExpr(cfg.Styling[r.ID], fieldKey)
		// Wrap inner ternaries in parens to keep precedence sane when
		// nested.
		if i < len(cfg.Rules)-1 {
			tail = "(" + tail + ")"
		}
		tail = fmt.Sprintf("%s ? %s : %s", pred, thenExpr, tail)
	}
	return tail, nil
}

// defaultExpr returns the no-rule-match fallback: either an outcome
// map literal (when the user customised default styling) or a bare
// field reference (so the engine's normalize emits {Text: <value>}).
func defaultExpr(d Outcome, fieldKey string) string {
	if outcomeIsEmpty(d) {
		return fieldKey
	}
	return outcomeExprFromOutcome(d, fieldKey)
}

// outcomeExpr emits a rule's then-branch. We always emit a map
// literal (never a bare field reference) so the rule's match
// produces a SidebarItem with at least the field's value as text.
func outcomeExpr(o Outcome, fieldKey string) string {
	return outcomeExprFromOutcome(o, fieldKey)
}

func outcomeExprFromOutcome(o Outcome, fieldKey string) string {
	parts := []string{}
	if o.Text != "" {
		parts = append(parts, "text: "+jsonString(o.Text))
	} else {
		parts = append(parts, "text: "+fieldKey)
	}
	if o.Color != "" {
		parts = append(parts, "color: "+jsonString(o.Color))
	}
	if o.Bg != "" {
		parts = append(parts, "bg: "+jsonString(o.Bg))
	}
	if len(o.Classes) > 0 {
		quoted := make([]string, len(o.Classes))
		for i, c := range o.Classes {
			quoted[i] = jsonString(c)
		}
		parts = append(parts, "classes: ["+strings.Join(quoted, ", ")+"]")
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func outcomeIsEmpty(o Outcome) bool {
	return o.Text == "" && o.Color == "" && o.Bg == "" && len(o.Classes) == 0
}

// predicate emits the boolean expr-lang fragment for a rule. Pointer
// fields (BoolValue, NumberValue) are required for kinds that use
// them — the JSON-shaped omitempty contract means a frontend payload
// missing them is malformed, not zero.
func predicate(r Rule, fieldKey string) (string, error) {
	switch r.Kind {
	case KindBoolean:
		if r.BoolValue == nil {
			return "", fmt.Errorf("builder: boolean rule %q missing value", r.ID)
		}
		if *r.BoolValue {
			return fieldKey, nil
		}
		return "!" + fieldKey, nil

	case KindEnum:
		if len(r.EnumValues) == 0 {
			return "", fmt.Errorf("builder: enum rule %q has no values", r.ID)
		}
		var op, join string
		switch r.EnumOp {
		case EnumOpEquals:
			op, join = "==", " || "
		case EnumOpNotEquals:
			op, join = "!=", " && "
		default:
			return "", fmt.Errorf("builder: enum rule %q has invalid op %q", r.ID, r.EnumOp)
		}
		terms := make([]string, len(r.EnumValues))
		for i, v := range r.EnumValues {
			terms[i] = fmt.Sprintf("%s %s %s", fieldKey, op, jsonString(v))
		}
		if len(terms) == 1 {
			return terms[0], nil
		}
		return "(" + strings.Join(terms, join) + ")", nil

	case KindNumber:
		if r.NumberValue == nil {
			return "", fmt.Errorf("builder: number rule %q missing value", r.ID)
		}
		switch r.NumberOp {
		case NumberOpEq, NumberOpNe, NumberOpGt, NumberOpGe, NumberOpLt, NumberOpLe:
			// ok
		default:
			return "", fmt.Errorf("builder: number rule %q has invalid op %q", r.ID, r.NumberOp)
		}
		return fmt.Sprintf("%s %s %s", fieldKey, r.NumberOp, formatFloat(*r.NumberValue)), nil

	case KindDate:
		if r.DateOp == "" {
			return "", fmt.Errorf("builder: date rule %q missing op", r.ID)
		}
		if r.DateArg != nil {
			return fmt.Sprintf("%s(%s, %d)", r.DateOp, fieldKey, *r.DateArg), nil
		}
		return fmt.Sprintf("%s(%s)", r.DateOp, fieldKey), nil
	}
	return "", fmt.Errorf("builder: unknown rule kind %q", r.Kind)
}

// jsonString quotes a string with JSON's escaping rules. We need
// embedded double-quotes/newlines/backslashes escaped so the emitted
// expr-lang source still parses; encoding/json's string encoder does
// exactly this so we reuse it instead of hand-rolling.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		// json.Marshal of a string only fails on invalid UTF-8; fall
		// back to a strconv.Quote which uses Go's escape rules — close
		// enough for the rare invalid-UTF-8 case.
		return strconv.Quote(s)
	}
	return string(b)
}

// formatFloat trims trailing zeros so 10.0 → "10" and 2.5 → "2.5".
// Avoids leaking float-formatting noise into the generated source.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
