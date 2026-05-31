package builder

import (
	"fmt"
	"strings"
)

// KindForField maps a field type to its RuleKind; ("", false) for types that accept no predicates.
func KindForField(fieldType string) (RuleKind, bool) {
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "boolean":
		return KindBoolean, true
	case "dropdown", "radio", "facet":
		return KindEnum, true
	case "number", "range":
		return KindNumber, true
	case "date":
		return KindDate, true
	}
	return "", false
}

// DefaultPredicateForField returns a freshly-initialised Predicate
// targeting the given field. Boolean defaults to "is true"; enum to
// "equals" with no values yet; number to "== 0"; date to "isOverdue".
func DefaultPredicateForField(fieldType, fieldKey string) (Predicate, error) {
	kind, ok := KindForField(fieldType)
	if !ok {
		return Predicate{}, fmt.Errorf("builder: field type %q does not support predicates", fieldType)
	}
	if strings.TrimSpace(fieldKey) == "" {
		return Predicate{}, fmt.Errorf("builder: predicate requires a non-empty field key")
	}
	p := Predicate{Kind: kind, FieldKey: fieldKey}
	switch kind {
	case KindBoolean:
		t := true
		p.BoolValue = &t
	case KindEnum:
		p.EnumOp = EnumOpEquals
		p.EnumValues = []string{}
	case KindNumber:
		var z float64
		p.NumberOp = NumberOpEq
		p.NumberValue = &z
	case KindDate:
		p.DateOp = DateOpIsOverdue
	}
	return p, nil
}

// DefaultRule is an empty Rule (no predicates, empty outcome); the frontend assigns the ID.
func DefaultRule() Rule {
	return Rule{
		Predicates: []Predicate{},
		Outcome:    Outcome{},
	}
}

// DefaultConfig is the empty config the dialog opens with; Compile returns "" until something is added.
func DefaultConfig() Config {
	return Config{
		Rules:   []Rule{},
		Default: Outcome{},
	}
}
