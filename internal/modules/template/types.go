// Package template owns Formidable's templates: schema-driven YAML files
// at <context>/templates/<name>.yaml that declare a form's fields.
//
// Mirrors `controls/templateManager.js` semantics. Loop
// pairing/nesting validation (max depth 2), single tags-field rule,
// api-field shape rules, collection-mode requires a guid field.
package template

import "strings"

// Template is the on-disk shape of a template YAML file.
type Template struct {
	Name              string  `yaml:"name" json:"name"`
	Filename          string  `yaml:"filename" json:"filename"`
	ItemField         string  `yaml:"item_field,omitempty" json:"item_field"`
	MarkdownTemplate  string  `yaml:"markdown_template,omitempty" json:"markdown_template"`
	SidebarExpression string  `yaml:"sidebar_expression,omitempty" json:"sidebar_expression"`
	EnableCollection  bool    `yaml:"enable_collection,omitempty" json:"enable_collection"`
	Fields            []Field `yaml:"fields" json:"fields"`
}

// Field describes one input in a template. Type-specific properties
// (run_mode, options, collection, etc.) sit alongside the common ones —
// downstream consumers ignore irrelevant fields.
type Field struct {
	// Common
	Key            string  `yaml:"key" json:"key"`
	Type           string  `yaml:"type" json:"type"`
	Label          string  `yaml:"label,omitempty" json:"label"`
	Description    string  `yaml:"description,omitempty" json:"description"`
	SummaryField   string  `yaml:"summary_field,omitempty" json:"summary_field,omitempty"`
	ExpressionItem bool    `yaml:"expression_item,omitempty" json:"expression_item"`
	TwoColumn      bool    `yaml:"two_column,omitempty" json:"two_column"`
	Collapsible    *bool   `yaml:"collapsible,omitempty" json:"collapsible,omitempty"`
	Readonly       bool    `yaml:"readonly,omitempty" json:"readonly"`
	Default        any     `yaml:"default,omitempty" json:"default"`
	Options        []any   `yaml:"options,omitempty" json:"options"`
	PrimaryKey     bool    `yaml:"primary_key,omitempty" json:"primary_key,omitempty"`

	// textarea-specific
	Format string `yaml:"format,omitempty" json:"format,omitempty"`

	// api-specific
	Collection string   `yaml:"collection,omitempty" json:"collection,omitempty"`
	ID         string   `yaml:"id,omitempty" json:"id,omitempty"`
	Map        []APIMap `yaml:"map,omitempty" json:"map,omitempty"`
	UsePicker  *bool    `yaml:"use_picker,omitempty" json:"use_picker,omitempty"`
	AllowedIDs []string `yaml:"allowed_ids,omitempty" json:"allowed_ids,omitempty"`

	// Extra fields preserved verbatim (e.g. plugin-specific metadata).
	Extra map[string]any `yaml:",inline" json:"-"`
}

// APIMap is one entry in an api field's map[].
type APIMap struct {
	Key  string `yaml:"key" json:"key"`
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
}

// ValidationError is one issue found by Validate.
type ValidationError struct {
	Type    string         `json:"type"`
	Message string         `json:"message,omitempty"`
	Key     string         `json:"key,omitempty"`
	Keys    []string       `json:"keys,omitempty"`
	Field   *Field         `json:"field,omitempty"`
	Index   int            `json:"index,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// ValidationFailedError wraps a slice of ValidationError. SaveTemplate
// returns this when validation finds issues so programmatic callers can
// errors.As to the structured set; the Wails layer just relays Error()
// to the frontend, which has its own pre-validation gate.
type ValidationFailedError struct {
	Errors []ValidationError
}

func (e *ValidationFailedError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "template: validation failed"
	}
	parts := make([]string, 0, len(e.Errors))
	for _, ve := range e.Errors {
		if ve.Message != "" {
			parts = append(parts, ve.Message)
		} else {
			parts = append(parts, ve.Type)
		}
	}
	return "template: validation failed: " + joinSemicolon(parts)
}

func joinSemicolon(parts []string) string {
	return strings.Join(parts, "; ")
}

// Descriptor is the {name, yaml, storageLocation} bundle returned by
// GetDescriptor. Mirrors templateManager.getTemplateDescriptor.
type Descriptor struct {
	Name            string    `json:"name"`
	YAML            *Template `json:"yaml"`
	StorageLocation string    `json:"storageLocation"`
}

// ItemField is one row in the "possible item fields" picker (top-level
// non-loop text fields, used to choose a collection's primary identifier).
type ItemField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Known field types live in field_registry.go's fieldTypeRegistry.
// Use IsKnownFieldType(t) to check membership.
