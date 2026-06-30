package template

// Field-attribute matrix. Adding an attribute means: a constant here, a bool in Abilities,
// a case each in propertyIsSet/clearProperty, and every type's entry in field_abilities.go.

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

// Abilities is the per-type ability vector; each bool gates one attribute in the modal and in
// save-time enforcement (false: row hidden, Normalize strips, validator flags any non-zero).
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

// FieldDescriptor is the per-type record. MetaOnly marks value-less marker types (loopstart/loopstop).
// Virtual marks types that don't seed a storage.Form.Data slot (their value lives elsewhere, e.g. meta.facets).
// OptionsShape is non-nil when the options array has fixed arity (e.g. boolean = two rows).
// RequiresCollection marks types that only mean something on a collection (sequence orders a record
// set); the editor's type dropdown hides them until Enable Collection is on, matching the validator.
type FieldDescriptor struct {
	ID                 string             `json:"id"`
	LabelKey           string             `json:"label_key"`
	MetaOnly           bool               `json:"meta_only"`
	Virtual            bool               `json:"virtual"`
	KeyReadonly        bool               `json:"key_readonly"`
	RequiresCollection bool               `json:"requires_collection"`
	Abilities          Abilities          `json:"abilities"`
	OptionsShape       *FixedOptionsShape `json:"options_shape,omitempty"`
	DefaultValue       any                `json:"default_value"`
}

// IsKnownFieldType reports whether the given type id is in the matrix.
func IsKnownFieldType(t string) bool {
	_, ok := fieldDescriptors[t]
	return ok
}

// IsVirtualFieldType reports whether the type is virtual; storage.Sanitize skips these when seeding data.
func IsVirtualFieldType(t string) bool {
	def, ok := fieldDescriptors[t]
	return ok && def.Virtual
}

// AllFieldTypes returns the matrix as a slice in orderedTypes order.
func AllFieldTypes() []FieldDescriptor {
	out := make([]FieldDescriptor, 0, len(orderedTypes))
	for _, id := range orderedTypes {
		def, ok := fieldDescriptors[id]
		if !ok {
			continue
		}
		def.LabelKey = fieldTypeLabelKeys[id]
		def.DefaultValue = fieldTypeDefaults[id]
		out = append(out, def)
	}
	return out
}

// allEnforcedAttrs is every attr that save-validation and Normalize iterate over (excludes always-present key/type).
var allEnforcedAttrs = []string{
	attrLabel, attrDescription, attrDefault, attrOptions, attrSummaryField,
	attrPrimaryKey, attrExpressionItem, attrTwoColumn, attrCollapsible,
	attrReadonly, attrFormat, attrUseInStatistics, attrFacetKey,
}

// abilityFor returns the Abilities bool for attr; unrecognized names return true so unmodeled attrs aren't stripped.
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
