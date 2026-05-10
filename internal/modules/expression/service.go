package expression

import "github.com/petervdpas/formidable2/internal/modules/expression/builder"

// Service is the Wails-bound facade for the expression module. Vue
// calls Evaluate for one-off expressions (e.g. plugin commands or a
// hypothetical preview pane in the template editor) and
// EvaluateSidebar to populate the Storage workspace's per-row
// sub-labels. Builder* methods power the visual sidebar-expression
// dialog by returning the same construction primitives the Go side
// uses internally — single source of truth.
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
// "" when the type does not participate in rules. Frontend uses this
// to gate the State / Date tabs.
func (s *Service) BuilderKindForFieldType(fieldType string) string {
	if k, ok := builder.KindForField(fieldType); ok {
		return string(k)
	}
	return ""
}

// BuilderDefaultRule returns a freshly-initialised Rule for the given
// field type. The frontend assigns the ID; the returned Rule has an
// empty ID so it cannot accidentally be persisted as authoritative.
func (s *Service) BuilderDefaultRule(fieldType string) (builder.Rule, error) {
	return builder.DefaultRuleForField(fieldType)
}

// BuilderDefaultFieldConfig returns the empty per-field config the
// modal seeds for every expression-flagged field on open.
func (s *Service) BuilderDefaultFieldConfig() builder.FieldConfig {
	return builder.DefaultFieldConfig()
}

// BuilderOperatorsForKind returns the operator vocabulary for the
// State-tab picker. Empty for kinds with no picker (boolean, date).
func (s *Service) BuilderOperatorsForKind(kind string) []builder.Operator {
	return builder.OperatorsForKind(builder.RuleKind(kind))
}

// BuilderDateOps returns the date-helper vocabulary for the Date-tab
// picker, in render order.
func (s *Service) BuilderDateOps() []builder.DateOpDescriptor {
	return builder.DateOps()
}

// BuilderCompile turns a FieldConfig into the expr-lang source string
// the engine evaluates. Empty string means "field hidden from the
// sidebar"; an error means the config is malformed (missing values,
// unknown ops) and the frontend should keep the modal open.
func (s *Service) BuilderCompile(cfg builder.FieldConfig, fieldKey string) (string, error) {
	return builder.Compile(cfg, fieldKey)
}
