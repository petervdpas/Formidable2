package template

// Field-attribute matrix - the type and helper layer. The actual
// per-type Abilities map lives in field_abilities.go so the matrix
// stays grep-friendly. Adding/removing an attribute means: (1) one
// new constant here, (2) one new bool in Abilities, (3) one case
// each in propertyIsSet / clearProperty, (4) every type's entry in
// field_abilities.go gets the new bool set explicitly.

const (
	attrKey             = "key"
	attrType            = "type"
	attrLabel           = "label"
	attrDescription     = "description"
	attrDefault         = "default"
	attrOptions         = "options"
	attrSummaryField    = "summary_field"
	attrPrimaryKey      = "primary_key"
	attrExpressionItem  = "expression_item"
	attrTwoColumn       = "two_column"
	attrCollapsible     = "collapsible"
	attrReadonly        = "readonly"
	attrFormat          = "format"
	attrUseInStatistics = "use_in_statistics"
	attrFacetKey        = "facet_key"
)

// Abilities is the per-type ability vector. Each bool gates a single
// attribute in the field-edit modal AND in backend save-time
// enforcement. true = enabled (modal row visible, value preserved on
// save); false = disabled (row hidden, Normalize strips, validator
// flags any non-zero).
type Abilities struct {
	Key             bool `json:"key"`
	Type            bool `json:"type"`
	Label           bool `json:"label"`
	Description     bool `json:"description"`
	Default         bool `json:"default"`
	Options         bool `json:"options"`
	SummaryField    bool `json:"summary_field"`
	PrimaryKey      bool `json:"primary_key"`
	ExpressionItem  bool `json:"expression_item"`
	TwoColumn       bool `json:"two_column"`
	Collapsible     bool `json:"collapsible"`
	Readonly        bool `json:"readonly"`
	Format          bool `json:"format"`
	UseInStatistics bool `json:"use_in_statistics"`
	FacetKey        bool `json:"facet_key"`
}

// FieldDescriptor is the per-type record. MetaOnly flags marker types
// (looper, loopstart, loopstop) that don't carry a stored value but
// still participate in validation. Virtual flags types that participate
// in template layout + validation but do NOT seed a slot in
// storage.Form.Data; their value lives elsewhere (e.g. facet → meta.facets).
// OptionsShape is non-nil when the type's options array has a fixed
// arity (e.g. boolean = exactly two rows for the True/False labels) -
// the frontend's OptionsEditor gates add/remove on this and pre-fills
// with the supplied defaults.
type FieldDescriptor struct {
	ID           string             `json:"id"`
	MetaOnly     bool               `json:"meta_only"`
	Virtual      bool               `json:"virtual"`
	Abilities    Abilities          `json:"abilities"`
	OptionsShape *FixedOptionsShape `json:"options_shape,omitempty"`
}

// IsKnownFieldType reports whether the given type id is in the matrix.
func IsKnownFieldType(t string) bool {
	_, ok := fieldDescriptors[t]
	return ok
}

// IsVirtualFieldType reports whether the given type id is registered
// as a virtual field. Virtual fields do not occupy a slot in
// storage.Form.Data; storage.Sanitize uses this to skip them when
// seeding the data map.
func IsVirtualFieldType(t string) bool {
	def, ok := fieldDescriptors[t]
	return ok && def.Virtual
}

// AllFieldTypes returns the matrix as a slice in the stable order
// declared by `orderedTypes`. Used by Service.FieldTypes (the
// Wails-facing single source of truth).
func AllFieldTypes() []FieldDescriptor {
	out := make([]FieldDescriptor, 0, len(orderedTypes))
	for _, id := range orderedTypes {
		def, ok := fieldDescriptors[id]
		if !ok {
			continue
		}
		out = append(out, def)
	}
	return out
}

// allEnforcedAttrs lists every attr name that backend save-validation
// and Normalize iterate over. Excludes "key" and "type" - those are
// always present and don't have a corresponding Field-property check.
var allEnforcedAttrs = []string{
	attrLabel, attrDescription, attrDefault, attrOptions, attrSummaryField,
	attrPrimaryKey, attrExpressionItem, attrTwoColumn, attrCollapsible,
	attrReadonly, attrFormat, attrUseInStatistics, attrFacetKey,
}

// abilityFor returns the Abilities bool for a given attr name.
// Returns true (allowed) for unrecognized names so future attrs that
// the matrix doesn't yet model don't accidentally get stripped.
func (a Abilities) abilityFor(attr string) bool {
	switch attr {
	case attrKey:
		return a.Key
	case attrType:
		return a.Type
	case attrLabel:
		return a.Label
	case attrDescription:
		return a.Description
	case attrDefault:
		return a.Default
	case attrOptions:
		return a.Options
	case attrSummaryField:
		return a.SummaryField
	case attrPrimaryKey:
		return a.PrimaryKey
	case attrExpressionItem:
		return a.ExpressionItem
	case attrTwoColumn:
		return a.TwoColumn
	case attrCollapsible:
		return a.Collapsible
	case attrReadonly:
		return a.Readonly
	case attrFormat:
		return a.Format
	case attrUseInStatistics:
		return a.UseInStatistics
	case attrFacetKey:
		return a.FacetKey
	}
	return true
}

func defaultIsPopulated(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case string:
		return x != ""
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

// propertyIsSet reports whether `attr` has a non-zero value on f.
func propertyIsSet(f Field, attr string) bool {
	switch attr {
	case attrSummaryField:
		return f.SummaryField != ""
	case attrPrimaryKey:
		return f.PrimaryKey
	case attrLabel:
		return f.Label != ""
	case attrDescription:
		return f.Description != ""
	case attrDefault:
		return defaultIsPopulated(f.Default)
	case attrOptions:
		return len(f.Options) > 0
	case attrExpressionItem:
		return f.ExpressionItem
	case attrTwoColumn:
		return f.TwoColumn
	case attrCollapsible:
		return f.Collapsible != nil
	case attrReadonly:
		return f.Readonly
	case attrFormat:
		return f.Format != ""
	case attrUseInStatistics:
		return f.UseInStatistics
	case attrFacetKey:
		return f.FacetKey != ""
	}
	return false
}
