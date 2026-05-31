// Package builder generates an expr-lang source string from a strongly-typed Config the dialog edits.
// Rules walk top-to-bottom (first match wins) and map predicate conjunctions to styled chips; Default covers no-match.
package builder

// MaxConcatParts caps a single Outcome's `+`-joined TextSource parts; enforced in both Compile and Parse.
const MaxConcatParts = 10

type RuleKind string

const (
	KindBoolean RuleKind = "boolean"
	KindEnum    RuleKind = "enum"
	KindNumber  RuleKind = "number"
	KindDate    RuleKind = "date"
)

type EnumOp string

const (
	EnumOpEquals    EnumOp = "equals"
	EnumOpNotEquals EnumOp = "not_equals"
)

type NumberOp string

const (
	NumberOpEq NumberOp = "=="
	NumberOpNe NumberOp = "!="
	NumberOpGt NumberOp = ">"
	NumberOpGe NumberOp = ">="
	NumberOpLt NumberOp = "<"
	NumberOpLe NumberOp = "<="
)

type DateOp string

const (
	DateOpIsOverdue        DateOp = "isOverdue"
	DateOpIsToday          DateOp = "isToday"
	DateOpIsFuture         DateOp = "isFuture"
	DateOpIsDueSoon        DateOp = "isDueSoon"
	DateOpIsOverdueInDays  DateOp = "isOverdueInDays"
	DateOpIsExpiredAfter   DateOp = "isExpiredAfter"
	DateOpIsUpcomingBefore DateOp = "isUpcomingBefore"
	DateOpDateGt           DateOp = "dateGt"
	DateOpDateLt           DateOp = "dateLt"
)

// Predicate is one kind-specific test against one field; pointer fields are used where the zero value is meaningful.
type Predicate struct {
	Kind     RuleKind `json:"kind"`
	FieldKey string   `json:"fieldKey"`

	BoolValue *bool `json:"boolValue,omitempty"`

	EnumOp     EnumOp   `json:"enumOp,omitempty"`
	EnumValues []string `json:"enumValues,omitempty"`

	NumberOp    NumberOp `json:"numberOp,omitempty"`
	NumberValue *float64 `json:"numberValue,omitempty"`

	DateOp  DateOp `json:"dateOp,omitempty"`
	DateArg *int   `json:"dateArg,omitempty"`
}

type TextKind string

const (
	TextKindLiteral    TextKind = "literal"
	TextKindFieldValue TextKind = "fieldValue"
	TextKindFieldLabel TextKind = "fieldLabel"
)

// TextSource decides the chip text: Literal is verbatim, FieldValue is the live value, FieldLabel is the option label.
type TextSource struct {
	Kind     TextKind `json:"kind"`
	Value    string   `json:"value,omitempty"`
	FieldKey string   `json:"fieldKey,omitempty"`
}

// Outcome is the styled chip a Rule (or default) produces. Parts (the preferred form) is a `+`-joined
// list of TextSources; Text is the legacy single-source fallback Compile reads when Parts is empty.
type Outcome struct {
	Text    *TextSource  `json:"text,omitempty"`
	Parts   []TextSource `json:"parts,omitempty"`
	Color   string       `json:"color,omitempty"`
	Bg      string       `json:"bg,omitempty"`
	Classes []string     `json:"classes,omitempty"`
}

// Rule is an AND of Predicates with one Outcome; empty Predicates always match.
type Rule struct {
	ID         string      `json:"id"`
	Predicates []Predicate `json:"predicates,omitempty"`
	Outcome    Outcome     `json:"outcome"`
}

// Config is the dialog-session state: Rules (first match wins) plus the no-match Default.
type Config struct {
	Rules   []Rule  `json:"rules,omitempty"`
	Default Outcome `json:"default"`
}

// FieldOption is one dropdown/radio option; Compile bakes fieldLabel lookups from it.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FieldRef is the slim per-field shape Compile needs (Key, Type for validation, Options for fieldLabel baking).
type FieldRef struct {
	Key     string        `json:"key"`
	Type    string        `json:"type"`
	Options []FieldOption `json:"options,omitempty"`
}
