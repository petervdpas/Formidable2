package builder

import (
	"fmt"
	"strings"
)

// KindForField maps a Field.Type string to its RuleKind. Returns
// ("", false) for any field type that does NOT participate in
// predicates (text, list, path, guid…) - callers gate the State /
// Date pickers on ok=false rather than guess a kind.
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

// DefaultRule is an empty Rule - no predicates (so it always matches
// if reached) and an empty outcome. Frontend assigns the ID after
// the call.
func DefaultRule() Rule {
	return Rule{
		Predicates: []Predicate{},
		Outcome:    Outcome{},
	}
}

// DefaultConfig is the empty config seeded when the dialog opens. No
// rules, no default-styling - Compile returns "" until the user adds
// something.
func DefaultConfig() Config {
	return Config{
		Rules:   []Rule{},
		Default: Outcome{},
	}
}
