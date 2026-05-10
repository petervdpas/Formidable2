// Package builder is the construction-side of the expression module.
// While `expression` evaluates an expr-lang source string against a
// record context, `builder` *generates* that source from a small,
// strongly-typed configuration the frontend dialog edits. The two
// halves share helper vocabulary (date helpers, etc.) but have no
// other coupling — Compile produces a plain string the Manager later
// hands to the engine.
package builder

// RuleKind discriminates the four rule shapes the dialog supports.
// One field type maps to exactly one kind (see KindForField); a rule
// stores its kind so render and compile switch directly on Rule.Kind
// without re-reading the field metadata.
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
	DateOpAgeGt            DateOp = "ageGt"
	DateOpAgeLt            DateOp = "ageLt"
)

// Rule is a single predicate in the rule list. Tagged-struct shape
// (rather than a sealed interface) so Wails-generated TS gets one
// concrete type per rule with optional fields — narrowing happens
// via Kind on the consumer side.
//
// Pointer fields exist where the zero value is meaningful: a boolean
// rule on `false`, a number rule on `0`, a date arg of `0` are all
// legitimate user choices, so omitempty alone cannot tell them apart
// from "field not set." Slices and string-typed enum fields keep the
// natural empty-is-unset contract.
type Rule struct {
	ID   string   `json:"id"`
	Kind RuleKind `json:"kind"`

	BoolValue *bool `json:"boolValue,omitempty"`

	EnumOp     EnumOp   `json:"enumOp,omitempty"`
	EnumValues []string `json:"enumValues,omitempty"`

	NumberOp    NumberOp `json:"numberOp,omitempty"`
	NumberValue *float64 `json:"numberValue,omitempty"`

	DateOp  DateOp `json:"dateOp,omitempty"`
	DateArg *int   `json:"dateArg,omitempty"`
}

// Outcome mirrors the rendered SidebarItem shape minus runtime-only
// fields. Compile emits an outcome as an expr-lang map literal whose
// keys match the SidebarItem JSON tags (text/color/bg/classes), so
// the engine's normalize() consumes it directly.
type Outcome struct {
	Text    string   `json:"text,omitempty"`
	Color   string   `json:"color,omitempty"`
	Bg      string   `json:"bg,omitempty"`
	Classes []string `json:"classes,omitempty"`
}

// FieldConfig is the per-field builder state for one expression-item
// field. Display gates the whole field; Rules are predicates from the
// State or Date tab; Styling[id] is the per-rule outcome from the
// Display tab; Default is the no-rule-match outcome. Transform lands
// in a later slice and isn't here yet.
type FieldConfig struct {
	Display bool               `json:"display"`
	Rules   []Rule             `json:"rules,omitempty"`
	Styling map[string]Outcome `json:"styling,omitempty"`
	Default Outcome            `json:"default"`
}
