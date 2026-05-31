package builder

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Compile turns a Config into expr-lang source: rules top-to-bottom (first match wins) ending in the default
// Outcome. fields validates predicate keys and bakes fieldLabel lookups. Empty Config compiles to "".
func Compile(cfg Config, fields []FieldRef) (string, error) {
	if len(cfg.Rules) == 0 && outcomeIsEmpty(cfg.Default) {
		return "", nil
	}

	idx := indexFields(fields)

	tail, err := outcomeExpr(cfg.Default, idx)
	if err != nil {
		return "", fmt.Errorf("builder: default outcome: %w", err)
	}

	for i := len(cfg.Rules) - 1; i >= 0; i-- {
		r := cfg.Rules[i]
		match, err := rulePredicate(r, idx)
		if err != nil {
			return "", fmt.Errorf("builder: rule %q: %w", r.ID, err)
		}
		thenExpr, err := outcomeExpr(r.Outcome, idx)
		if err != nil {
			return "", fmt.Errorf("builder: rule %q outcome: %w", r.ID, err)
		}
		if i < len(cfg.Rules)-1 {
			tail = "(" + tail + ")"
		}
		tail = fmt.Sprintf("%s ? %s : %s", match, thenExpr, tail)
	}
	return tail, nil
}

func indexFields(fields []FieldRef) map[string]FieldRef {
	m := make(map[string]FieldRef, len(fields))
	for _, f := range fields {
		m[f.Key] = f
	}
	return m
}

// rulePredicate joins the rule's Predicates with AND; empty Predicates returns "true" (always matches).
func rulePredicate(r Rule, fields map[string]FieldRef) (string, error) {
	if len(r.Predicates) == 0 {
		return "true", nil
	}
	parts := make([]string, len(r.Predicates))
	for i, p := range r.Predicates {
		s, err := predicateExpr(p, fields)
		if err != nil {
			return "", err
		}
		parts[i] = s
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return "(" + strings.Join(parts, " && ") + ")", nil
}

func predicateExpr(p Predicate, fields map[string]FieldRef) (string, error) {
	key := strings.TrimSpace(p.FieldKey)
	if key == "" {
		return "", fmt.Errorf("predicate has no field key")
	}
	// FieldRef is required to validate the predicate kind against the field's declared type.
	f, ok := fields[key]
	if !ok {
		return "", fmt.Errorf("predicate references unknown field %q", key)
	}
	wantKind, kindOK := KindForField(f.Type)
	if !kindOK {
		return "", fmt.Errorf("predicate field %q has type %q which does not support predicates", key, f.Type)
	}
	if p.Kind != wantKind {
		return "", fmt.Errorf("predicate on field %q has kind %q but field type %q expects %q", key, p.Kind, f.Type, wantKind)
	}

	ref := fieldRef(key)
	switch p.Kind {
	case KindBoolean:
		if p.BoolValue == nil {
			return "", fmt.Errorf("boolean predicate on %q missing value", key)
		}
		if *p.BoolValue {
			return ref, nil
		}
		return "!" + ref, nil

	case KindEnum:
		if len(p.EnumValues) == 0 {
			return "", fmt.Errorf("enum predicate on %q has no values", key)
		}
		var op, join string
		switch p.EnumOp {
		case EnumOpEquals:
			op, join = "==", " || "
		case EnumOpNotEquals:
			op, join = "!=", " && "
		default:
			return "", fmt.Errorf("enum predicate on %q has invalid op %q", key, p.EnumOp)
		}
		terms := make([]string, len(p.EnumValues))
		for i, v := range p.EnumValues {
			terms[i] = fmt.Sprintf("%s %s %s", ref, op, jsonString(v))
		}
		if len(terms) == 1 {
			return terms[0], nil
		}
		return "(" + strings.Join(terms, join) + ")", nil

	case KindNumber:
		if p.NumberValue == nil {
			return "", fmt.Errorf("number predicate on %q missing value", key)
		}
		switch p.NumberOp {
		case NumberOpEq, NumberOpNe, NumberOpGt, NumberOpGe, NumberOpLt, NumberOpLe:
		default:
			return "", fmt.Errorf("number predicate on %q has invalid op %q", key, p.NumberOp)
		}
		return fmt.Sprintf("%s %s %s", ref, p.NumberOp, formatFloat(*p.NumberValue)), nil

	case KindDate:
		if p.DateOp == "" {
			return "", fmt.Errorf("date predicate on %q missing op", key)
		}
		switch p.DateOp {
		case DateOpDateGt:
			if p.DateArg == nil {
				return "", fmt.Errorf("date predicate on %q missing arg for %s", key, p.DateOp)
			}
			return fmt.Sprintf("ageInDays(%s) > %d", ref, *p.DateArg), nil
		case DateOpDateLt:
			if p.DateArg == nil {
				return "", fmt.Errorf("date predicate on %q missing arg for %s", key, p.DateOp)
			}
			return fmt.Sprintf("ageInDays(%s) < %d", ref, *p.DateArg), nil
		}
		if p.DateArg != nil {
			return fmt.Sprintf("%s(%s, %d)", p.DateOp, ref, *p.DateArg), nil
		}
		return fmt.Sprintf("%s(%s)", p.DateOp, ref), nil
	}
	return "", fmt.Errorf("unknown predicate kind %q", p.Kind)
}

// outcomeExpr emits a Result-shaped map literal; an empty outcome is "{}" (caller skips no-ops via outcomeIsEmpty).
func outcomeExpr(o Outcome, fields map[string]FieldRef) (string, error) {
	parts := []string{}
	if text, err := textPartsExpr(o, fields); err != nil {
		return "", err
	} else if text != "" {
		parts = append(parts, "text: "+text)
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
	return "{" + strings.Join(parts, ", ") + "}", nil
}

// textPartsExpr produces an outcome's text: RHS; Parts wins over Text (Text is the legacy path). Empty returns "".
func textPartsExpr(o Outcome, fields map[string]FieldRef) (string, error) {
	if len(o.Parts) > MaxConcatParts {
		return "", fmt.Errorf("text has %d parts, max is %d", len(o.Parts), MaxConcatParts)
	}
	if len(o.Parts) > 0 {
		pieces := make([]string, len(o.Parts))
		for i, p := range o.Parts {
			s, err := textExpr(p, fields)
			if err != nil {
				return "", fmt.Errorf("part %d: %w", i+1, err)
			}
			pieces[i] = s
		}
		return strings.Join(pieces, " + "), nil
	}
	if o.Text != nil {
		return textExpr(*o.Text, fields)
	}
	return "", nil
}

// textExpr resolves a TextSource to its fragment: literal->L["text"], fieldValue->F["key"], fieldLabel->O["key"].
// All three share a uniform accessor["arg"] shape so a concat chain parses as a flat sequence of MemberNodes.
func textExpr(ts TextSource, fields map[string]FieldRef) (string, error) {
	switch ts.Kind {
	case TextKindLiteral:
		return literalRef(ts.Value), nil
	case TextKindFieldValue:
		if strings.TrimSpace(ts.FieldKey) == "" {
			return "", fmt.Errorf("fieldValue text source missing fieldKey")
		}
		return fieldRef(ts.FieldKey), nil
	case TextKindFieldLabel:
		key := strings.TrimSpace(ts.FieldKey)
		if key == "" {
			return "", fmt.Errorf("fieldLabel text source missing fieldKey")
		}
		// No-options fallback: a fieldLabel on an optionless field degrades to the raw value, not a runtime nil.
		if f, ok := fields[key]; !ok || len(f.Options) == 0 {
			return fieldRef(key), nil
		}
		return optionLabelRef(key), nil
	}
	return "", fmt.Errorf("unknown text source kind %q", ts.Kind)
}

func outcomeIsEmpty(o Outcome) bool {
	return o.Text == nil && len(o.Parts) == 0 && o.Color == "" && o.Bg == "" && len(o.Classes) == 0
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return string(b)
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// fieldRef emits F["key"]; the engine's patcher rewrites it to $env["key"] so hyphenated and plain keys behave alike.
func fieldRef(key string) string {
	return `F[` + jsonString(key) + `]`
}

// literalRef wraps a string in L["..."] so the parser can identify literal concat parts by AST shape.
func literalRef(s string) string {
	return `L[` + jsonString(s) + `]`
}

// optionLabelRef emits O["key"], resolved at runtime to the field's current value's option label.
func optionLabelRef(key string) string {
	return `O[` + jsonString(key) + `]`
}
