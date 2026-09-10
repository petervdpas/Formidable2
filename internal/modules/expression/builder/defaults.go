package builder

import (
	"fmt"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// KindForField maps a field type to its RuleKind; ("", false) for types that accept no predicates.
func KindForField(fieldType string) (RuleKind, bool) {
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "boolean":
		return KindBoolean, true
	case "dropdown", "radio", "facet":
		return KindEnum, true
	case "number", "range", "sequence":
		return KindNumber, true
	case "date":
		return KindDate, true
	}
	return "", false
}

// KindForFormula maps a formula's declared result type to its RuleKind;
// ("", false) for types that accept no predicates. A blank type resolves
// through template.EffectiveFormulaType, so an untyped formula behaves the
// same here as it does everywhere else. "text" has no kind, matching the text
// field type: the builder has no string comparison vocabulary.
func KindForFormula(formulaType string) (RuleKind, bool) {
	switch template.EffectiveFormulaType(strings.ToLower(formulaType)) {
	case "number":
		return KindNumber, true
	case "bool":
		return KindBoolean, true
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
	return defaultPredicateForKind(kind, fieldKey)
}

// DefaultPredicateForFormula is DefaultPredicateForField's counterpart for a
// formula key; the resulting Predicate is shape-identical, since a formula
// compiles to the same F["key"] accessor a field does.
func DefaultPredicateForFormula(formulaType, formulaKey string) (Predicate, error) {
	kind, ok := KindForFormula(formulaType)
	if !ok {
		return Predicate{}, fmt.Errorf("builder: formula type %q does not support predicates", formulaType)
	}
	return defaultPredicateForKind(kind, formulaKey)
}

func defaultPredicateForKind(kind RuleKind, key string) (Predicate, error) {
	if strings.TrimSpace(key) == "" {
		return Predicate{}, fmt.Errorf("builder: predicate requires a non-empty field key")
	}
	p := Predicate{Kind: kind, FieldKey: key}
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
