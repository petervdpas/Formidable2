package expression

import (
	"github.com/petervdpas/formidable2/internal/modules/expression/builder"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Service is the Wails facade for the expression module; Builder* methods power the visual dialog.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Evaluate runs one expression against an arbitrary context.
func (s *Service) Evaluate(src string, ctx map[string]any) (Result, error) {
	return s.m.Evaluate(src, ctx)
}

// EvaluateList renders the sub-label for every record; ErrNoExpression when no sidebar_expression is configured.
func (s *Service) EvaluateList(templateName string) ([]Result, error) {
	return s.m.EvaluateList(templateName)
}

// EvaluateListOne renders the sub-label for one record (a row refreshing itself after a save).
func (s *Service) EvaluateListOne(templateName, datafile string) (Result, error) {
	return s.m.EvaluateListOne(templateName, datafile)
}

// EvaluateListMany renders sub-labels for an explicit ordered list of records in one round-trip.
func (s *Service) EvaluateListMany(templateName string, datafiles []string) ([]Result, error) {
	return s.m.EvaluateListMany(templateName, datafiles)
}

// Functions returns the formula editor's function/control catalog so the
// palettes reflect the engine's real capabilities.
func (s *Service) Functions() []FunctionDoc { return Functions() }

// BuilderKindForFieldType reports the rule kind for a field type, or "" when it accepts no predicates.
func (s *Service) BuilderKindForFieldType(fieldType string) string {
	if k, ok := builder.KindForField(fieldType); ok {
		return string(k)
	}
	return ""
}

// BuilderIsDisplayableFieldType reports whether a type may appear in the display pickers; virtual types return false.
func (s *Service) BuilderIsDisplayableFieldType(fieldType string) bool {
	return builder.IsDisplayableFieldType(fieldType)
}

// BuilderTextSources returns the field-value sources an OUTCOME text part may
// use: the displayable fields (by type) followed by the template's formula
// fields. The backend decides what is selectable; the editor renders the list.
// A formula compiles to F["key"] and resolves at runtime from the harvested
// expression context, so it is just another field-value source here.
func (s *Service) BuilderTextSources(fields []template.Field, formulas []template.Formula) []builder.TextSourceOption {
	out := make([]builder.TextSourceOption, 0, len(fields)+len(formulas))
	for _, f := range fields {
		if builder.IsDisplayableFieldType(f.Type) {
			out = append(out, builder.TextSourceOption{Key: f.Key, Label: labelOr(f.Label, f.Key), Group: "field"})
		}
	}
	for _, fm := range formulas {
		out = append(out, builder.TextSourceOption{Key: fm.Key, Label: labelOr(fm.Label, fm.Key), Group: "formula"})
	}
	return out
}

func labelOr(label, key string) string {
	if label != "" {
		return label
	}
	return key
}

// BuilderFieldOptions returns the value/label pairs the value-picker offers, resolving virtual types (facet -> labels).
func (s *Service) BuilderFieldOptions(field template.Field, facets []template.Facet) []builder.FieldOption {
	return builder.OptionsForField(field, facets)
}

// BuilderDefaultPredicate returns a fresh Predicate targeting the given field.
func (s *Service) BuilderDefaultPredicate(fieldType, fieldKey string) (builder.Predicate, error) {
	return builder.DefaultPredicateForField(fieldType, fieldKey)
}

// BuilderDefaultRule returns an empty Rule; the frontend assigns the ID.
func (s *Service) BuilderDefaultRule() builder.Rule {
	return builder.DefaultRule()
}

// BuilderDefaultConfig returns the empty dialog-session config.
func (s *Service) BuilderDefaultConfig() builder.Config {
	return builder.DefaultConfig()
}

// BuilderOperatorsForKind returns the operator vocabulary for the State picker (empty for boolean/date).
func (s *Service) BuilderOperatorsForKind(kind string) []builder.Operator {
	return builder.OperatorsForKind(builder.RuleKind(kind))
}

// BuilderDateOps returns the date-helper vocabulary for the Date picker, in render order.
func (s *Service) BuilderDateOps() []builder.DateOpDescriptor {
	return builder.DateOps()
}

// BuilderCompile turns a Config into the expr-lang source; "" means "no chip", an error keeps the dialog open.
func (s *Service) BuilderCompile(cfg builder.Config, fields []builder.FieldRef) (string, error) {
	return builder.Compile(cfg, fields)
}

// BuilderParse is the inverse of BuilderCompile; non-canonical sources error so the dialog can fall back.
func (s *Service) BuilderParse(src string, fields []builder.FieldRef) (builder.Config, error) {
	return builder.Parse(src, fields)
}

// BuilderConvert best-effort migrates a legacy sidebar_expression, invoked only when BuilderParse fails.
func (s *Service) BuilderConvert(src string, fields []builder.FieldRef) (string, error) {
	return builder.Convert(src, fields)
}
