package builder

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Compile turns a Config into the expr-lang source string the engine
// evaluates against a record's harvested ExpressionItems. Walks rules
// top-to-bottom (first match wins), with the default Outcome as the
// terminal else.
//
// fields is the FieldRef slice for every expression_item field in
// the template — Compile uses it to validate predicate field keys
// and to bake fieldLabel TextSources into value→label ternary
// lookups. Frontend supplies it from template metadata.
//
// Empty Config (no rules + empty default) compiles to "" so the
// frontend can hide the chip entirely. A non-empty Compile output
// is always a single expr-lang expression — either an outcome map
// literal or a ternary chain ending in one.
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

// rulePredicate joins the rule's Predicates with logical AND. Empty
// Predicates returns "true" so the rule always matches — useful for
// a lone-rule case where the user wants exactly one outcome.
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
	// FieldRef is required so we can validate predicate kinds against
	// the field's declared type. Missing entry usually means the
	// frontend forgot to include the field in the FieldRef slice.
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

	switch p.Kind {
	case KindBoolean:
		if p.BoolValue == nil {
			return "", fmt.Errorf("boolean predicate on %q missing value", key)
		}
		if *p.BoolValue {
			return key, nil
		}
		return "!" + key, nil

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
			terms[i] = fmt.Sprintf("%s %s %s", key, op, jsonString(v))
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
			// ok
		default:
			return "", fmt.Errorf("number predicate on %q has invalid op %q", key, p.NumberOp)
		}
		return fmt.Sprintf("%s %s %s", key, p.NumberOp, formatFloat(*p.NumberValue)), nil

	case KindDate:
		if p.DateOp == "" {
			return "", fmt.Errorf("date predicate on %q missing op", key)
		}
		// dateGt / dateLt express "date is older / newer than N days"
		// in user-facing terms. There is no helper by that name; we
		// emit ageInDays(<key>) > N (and < N) which uses the engine's
		// real age helper. All other ops are direct helper calls.
		switch p.DateOp {
		case DateOpDateGt:
			if p.DateArg == nil {
				return "", fmt.Errorf("date predicate on %q missing arg for %s", key, p.DateOp)
			}
			return fmt.Sprintf("ageInDays(%s) > %d", key, *p.DateArg), nil
		case DateOpDateLt:
			if p.DateArg == nil {
				return "", fmt.Errorf("date predicate on %q missing arg for %s", key, p.DateOp)
			}
			return fmt.Sprintf("ageInDays(%s) < %d", key, *p.DateArg), nil
		}
		if p.DateArg != nil {
			return fmt.Sprintf("%s(%s, %d)", p.DateOp, key, *p.DateArg), nil
		}
		return fmt.Sprintf("%s(%s)", p.DateOp, key), nil
	}
	return "", fmt.Errorf("unknown predicate kind %q", p.Kind)
}

// outcomeExpr emits a SidebarItem-shaped map literal. An outcome
// with no styling and no text is "{}" — the engine renders an empty
// chip but the source stays parseable. Caller's job to skip emitting
// a no-op chip via outcomeIsEmpty.
func outcomeExpr(o Outcome, fields map[string]FieldRef) (string, error) {
	parts := []string{}
	if o.Text != nil {
		s, err := textExpr(*o.Text, fields)
		if err != nil {
			return "", err
		}
		parts = append(parts, "text: "+s)
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

// textExpr resolves a TextSource to its expr-lang fragment. Literal
// becomes a quoted string; fieldValue a bare identifier; fieldLabel
// a baked value→label ternary over the field's options, falling
// through to the raw value when no option matches.
func textExpr(ts TextSource, fields map[string]FieldRef) (string, error) {
	switch ts.Kind {
	case TextKindLiteral:
		return jsonString(ts.Value), nil
	case TextKindFieldValue:
		if strings.TrimSpace(ts.FieldKey) == "" {
			return "", fmt.Errorf("fieldValue text source missing fieldKey")
		}
		return ts.FieldKey, nil
	case TextKindFieldLabel:
		key := strings.TrimSpace(ts.FieldKey)
		if key == "" {
			return "", fmt.Errorf("fieldLabel text source missing fieldKey")
		}
		f, ok := fields[key]
		if !ok || len(f.Options) == 0 {
			// Graceful fallback: bare reference. UI gates fieldLabel
			// to enum fields, so this only fires on stale state.
			return key, nil
		}
		return bakeOptionLookup(key, f.Options), nil
	}
	return "", fmt.Errorf("unknown text source kind %q", ts.Kind)
}

// bakeOptionLookup emits a nested ternary that resolves the field's
// stored value to its option label. Unknown values fall through to
// the raw value so a stale option doesn't blank out a chip.
//
//   key == "v1" ? "L1" : (key == "v2" ? "L2" : key)
func bakeOptionLookup(key string, opts []FieldOption) string {
	tail := key
	for i := len(opts) - 1; i >= 0; i-- {
		opt := opts[i]
		if i < len(opts)-1 {
			tail = "(" + tail + ")"
		}
		tail = fmt.Sprintf("%s == %s ? %s : %s", key, jsonString(opt.Value), jsonString(opt.Label), tail)
	}
	return tail
}

func outcomeIsEmpty(o Outcome) bool {
	return o.Text == nil && o.Color == "" && o.Bg == "" && len(o.Classes) == 0
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
