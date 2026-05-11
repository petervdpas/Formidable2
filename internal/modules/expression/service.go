package expression

import "github.com/petervdpas/formidable2/internal/modules/expression/builder"

// Service is the Wails-bound facade for the expression module. Vue
// calls Evaluate for one-off expressions and EvaluateSidebar to
// populate the Storage workspace's per-row sub-labels. Builder*
// methods power the visual sidebar-expression dialog by returning
// the same construction primitives the Go side uses internally —
// backend is the source of truth.
type Service struct{ m *Manager }

// NewService wraps a Manager. Service stays thin so all behaviour
// (cache, helpers, narrow-context defence) lives on Manager — the
// Wails surface adds nothing beyond IDL-style passthrough.
func NewService(m *Manager) *Service { return &Service{m: m} }

// Evaluate runs one expression against an arbitrary context. Returns
// a normalised SidebarItem so callers get the same shape whether the
// expression returns a string, list, or struct.
func (s *Service) Evaluate(src string, ctx map[string]any) (SidebarItem, error) {
	return s.m.Evaluate(src, ctx)
}

// EvaluateSidebar renders the sub-label for every record in a
// template's storage list. Returns ErrNoExpression when the template
// has no sidebar_expression configured — the frontend should hide
// the sub-label entirely in that case rather than render anything.
func (s *Service) EvaluateSidebar(templateName string) ([]SidebarItem, error) {
	return s.m.EvaluateSidebar(templateName)
}

// BuilderKindForFieldType reports the rule kind for a Field.Type, or
// "" when the type does not participate in predicates. Frontend uses
// this to gate predicate construction (only state-bearing + date
// types accept predicates).
func (s *Service) BuilderKindForFieldType(fieldType string) string {
	if k, ok := builder.KindForField(fieldType); ok {
		return string(k)
	}
	return ""
}

// BuilderDefaultPredicate returns a freshly-initialised Predicate
// targeting the given field. The frontend supplies the field's type
// (to pick the kind) and key (the variable name in the expression).
func (s *Service) BuilderDefaultPredicate(fieldType, fieldKey string) (builder.Predicate, error) {
	return builder.DefaultPredicateForField(fieldType, fieldKey)
}

// BuilderDefaultRule returns an empty Rule (no predicates, empty
// outcome). Frontend assigns the ID after the call.
func (s *Service) BuilderDefaultRule() builder.Rule {
	return builder.DefaultRule()
}

// BuilderDefaultConfig returns the empty dialog-session config —
// no rules, empty default outcome. Compile produces "" until rules
// or default styling are added.
func (s *Service) BuilderDefaultConfig() builder.Config {
	return builder.DefaultConfig()
}

// BuilderOperatorsForKind returns the operator vocabulary for the
// State picker. Empty for kinds with no picker (boolean, date).
func (s *Service) BuilderOperatorsForKind(kind string) []builder.Operator {
	return builder.OperatorsForKind(builder.RuleKind(kind))
}

// BuilderDateOps returns the date-helper vocabulary for the Date
// picker, in render order.
func (s *Service) BuilderDateOps() []builder.DateOpDescriptor {
	return builder.DateOps()
}

// BuilderCompile turns a Config into the expr-lang source string the
// engine evaluates. fields is the FieldRef slice for every
// expression_item field — Compile uses it to validate predicates and
// to bake fieldLabel TextSources into value→label ternary lookups.
// Empty string means "no chip"; an error means the config is
// malformed and the dialog should keep itself open.
func (s *Service) BuilderCompile(cfg builder.Config, fields []builder.FieldRef) (string, error) {
	return builder.Compile(cfg, fields)
}

// BuilderParse is the inverse of BuilderCompile: an expr-lang source
// string that came from a previous Compile is reverse-engineered
// back into a Config the dialog can edit. Hand-authored or
// otherwise-shaped sources return an error so the frontend can fall
// back to an empty config and warn the user. fields is the same
// FieldRef slice Compile takes.
func (s *Service) BuilderParse(src string, fields []builder.FieldRef) (builder.Config, error) {
	return builder.Parse(src, fields)
}

// BuilderConvert is a best-effort migrator for legacy
// sidebar_expression shapes (array-wrapped ternaries, the old `|`
// pipe form, bare identifiers, bare string literals in text concats,
// `F[..] == true` boolean predicates). Frontend invokes it only when
// BuilderParse fails — the converted output is fed straight back into
// Parse → Compile so the dialog can edit a canonical DSL. Returns an
// error when the source can't be parsed even after the pre-pass; the
// frontend should surface the error and offer manual editing.
func (s *Service) BuilderConvert(src string, fields []builder.FieldRef) (string, error) {
	return builder.Convert(src, fields)
}
