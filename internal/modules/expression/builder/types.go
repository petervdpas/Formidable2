// Package builder is the construction-side of the expression module.
// While `expression` evaluates an expr-lang source string against a
// record context, `builder` *generates* that source from a small,
// strongly-typed configuration the dialog edits. The two halves
// share helper vocabulary (date helpers, etc.) but have no other
// coupling — Compile produces a plain string the Manager later
// hands to the engine.
//
// Mental model: the dialog is a small logic engine. Its inputs are
// the template's expression_item fields. Its rules are cross-field
// conjunctions of predicates that map to a styled chip. The rule
// list walks top-to-bottom (first match wins); the default outcome
// covers no-match.
package builder

// MaxConcatParts caps how many TextSource parts a single Outcome
// can `+`-join. Sidebar chips are short labels — going past this
// produces unreadable text and pathological compile/parse work.
// Both Compile and Parse enforce the cap so hand-authored sources
// bouncing through Parse → Compile can't smuggle larger chains in.
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

// Predicate is one kind-specific test against one expression_item
// field. A Rule's match clause ANDs all its Predicates together;
// pointer fields exist where the zero value is meaningful (a boolean
// predicate on `false`, a number on `0`, a date arg of `0`).
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

// TextSource decides what the chip's text resolves to. Literal renders
// `Value` verbatim. FieldValue emits a bare reference to FieldKey
// (engine evaluates the live value). FieldLabel emits a baked
// value→label ternary over the field's options, falling back to the
// raw value when no option matches.
type TextSource struct {
	Kind     TextKind `json:"kind"`
	Value    string   `json:"value,omitempty"`
	FieldKey string   `json:"fieldKey,omitempty"`
}

// Outcome is the styled chip a matching Rule (or the default)
// produces. Text and Parts both encode the chip text:
//
//   - Parts (preferred) is an ordered list of TextSources joined
//     with `+` so a chip text can mix literals, field values, and
//     option labels — e.g. `unit-number + " " + street`.
//   - Text is the legacy single-source form. Compile reads Parts
//     first; falls back to Text when Parts is empty. Parse always
//     emits Parts (Text stays nil). Both nil/empty means the chip
//     renders no text.
//
// The remaining fields mirror the runtime Result shape minus
// filename/error.
type Outcome struct {
	Text    *TextSource  `json:"text,omitempty"`
	Parts   []TextSource `json:"parts,omitempty"`
	Color   string       `json:"color,omitempty"`
	Bg      string       `json:"bg,omitempty"`
	Classes []string     `json:"classes,omitempty"`
}

// Rule is a logical AND of Predicates with one Outcome. Empty
// Predicates always match — useful for the lone-rule case where the
// user wants exactly one outcome regardless of state.
type Rule struct {
	ID         string      `json:"id"`
	Predicates []Predicate `json:"predicates,omitempty"`
	Outcome    Outcome     `json:"outcome"`
}

// Config is the dialog-session state. Rules are walked top-to-bottom;
// the first whose predicate clause holds wins. Default is the no-match
// fallback.
type Config struct {
	Rules   []Rule  `json:"rules,omitempty"`
	Default Outcome `json:"default"`
}

// FieldOption is one entry in a dropdown/radio field's options list.
// Compile reads it to bake fieldLabel TextSources at construction
// time so the engine doesn't need an option-lookup helper.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FieldRef is the slim shape Compile needs from each template field.
// Key is the variable name in the expression; Type lets Compile
// validate predicates against their declared kinds; Options drives
// fieldLabel ternary baking and is empty for non-enum fields.
type FieldRef struct {
	Key     string        `json:"key"`
	Type    string        `json:"type"`
	Options []FieldOption `json:"options,omitempty"`
}
