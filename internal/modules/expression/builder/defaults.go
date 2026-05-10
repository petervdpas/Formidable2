package builder

import (
	"fmt"
	"strings"
)

// KindForField maps a Field.Type string to its RuleKind. Returns
// ("", false) for any field type that does NOT participate in rules
// (text, list, path, guid…) — callers use ok=false to disable the
// State/Date tabs entirely rather than guess a kind.
func KindForField(fieldType string) (RuleKind, bool) {
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "boolean":
		return KindBoolean, true
	case "dropdown", "radio":
		return KindEnum, true
	case "number", "range":
		return KindNumber, true
	case "date":
		return KindDate, true
	}
	return "", false
}

// DefaultRuleForField returns a freshly-initialised Rule of the kind
// matching the given field type. Callers (the modal) assign IDs after
// the call — keeps this function deterministic and testable, and the
// frontend's session-scoped counter stays the only id authority.
func DefaultRuleForField(fieldType string) (Rule, error) {
	kind, ok := KindForField(fieldType)
	if !ok {
		return Rule{}, fmt.Errorf("builder: field type %q does not support rules", fieldType)
	}
	switch kind {
	case KindBoolean:
		t := true
		return Rule{Kind: KindBoolean, BoolValue: &t}, nil
	case KindEnum:
		return Rule{Kind: KindEnum, EnumOp: EnumOpEquals, EnumValues: []string{}}, nil
	case KindNumber:
		var z float64
		return Rule{Kind: KindNumber, NumberOp: NumberOpEq, NumberValue: &z}, nil
	case KindDate:
		return Rule{Kind: KindDate, DateOp: DateOpIsOverdue}, nil
	}
	return Rule{}, fmt.Errorf("builder: unhandled rule kind %q", kind)
}

// DefaultFieldConfig is the empty config seeded for every expression
// field on modal open. Slices/maps are non-nil so the frontend can
// mutate without nil checks.
func DefaultFieldConfig() FieldConfig {
	return FieldConfig{
		Display: false,
		Rules:   []Rule{},
		Styling: map[string]Outcome{},
		Default: Outcome{},
	}
}
