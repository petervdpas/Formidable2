package template

// Field-type registry. Mirrors `utils/fieldTypes.js` from the original
// Formidable: each entry declares which optional Field properties are
// FORBIDDEN for that type. The frontend's `field-types.ts` registry
// drives editor visibility for the same set of properties; here we
// enforce the contract at save-time so a hand-edited YAML or a
// programmatic writer can't leave meaningless data on a field.

// Property names used across the registry. Stable strings — referenced
// by tests and by the ValidationError detail. Group names (code/
// latex/api) check the union of their members.
const (
	attrSummaryField   = "summary_field"
	attrPrimaryKey     = "primary_key"
	attrLabel          = "label"
	attrDescription    = "description"
	attrDefault        = "default"
	attrOptions        = "options"
	attrExpressionItem = "expression_item"
	attrTwoColumn      = "two_column"
	attrCollapsible    = "collapsible"
	attrReadonly       = "readonly"
	attrFormat         = "format"
	attrCodeGroup      = "code"  // run_mode / allow_run / hide_field / input_mode / api_mode / api_pick
	attrLatexGroup     = "latex" // rows / use_fenced
	attrAPIGroup       = "api"   // collection / id / map / use_picker / allowed_ids
)

// FieldTypeDef declares a known field type and the optional Field
// properties it forbids. `MetaOnly` flags the marker types (looper,
// loopstart, loopstop) that don't carry a stored value but still
// participate in validation.
//
// JSON tags are present because FieldTypeDef is also the Wails-facing
// shape returned by Service.FieldTypes — the frontend uses it as the
// single source of truth for "what types exist and what they forbid".
type FieldTypeDef struct {
	ID                  string   `json:"id"`
	MetaOnly            bool     `json:"meta_only"`
	ForbiddenAttributes []string `json:"forbidden_attributes"`
}

// fieldTypeRegistry — mirrors original Formidable's `disabledAttributes`
// per type (utils/fieldTypes.js). Order within a type's list is not
// significant. Adding a new type is one entry plus a per-type test.
var fieldTypeRegistry = map[string]FieldTypeDef{
	"guid": {
		ID: "guid",
		ForbiddenAttributes: []string{
			attrPrimaryKey, attrLabel, attrDescription, attrDefault,
			attrOptions, attrSummaryField, attrExpressionItem, attrTwoColumn,
			attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"looper": {
		ID: "looper", MetaOnly: true,
		ForbiddenAttributes: []string{
			attrDefault, attrOptions, attrExpressionItem, attrTwoColumn,
			attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"loopstart": {
		ID: "loopstart", MetaOnly: true,
		ForbiddenAttributes: []string{
			attrDefault, attrOptions, attrExpressionItem, attrTwoColumn,
			attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"loopstop": {
		ID: "loopstop", MetaOnly: true,
		ForbiddenAttributes: []string{
			attrSummaryField, attrDescription, attrDefault, attrOptions,
			attrExpressionItem, attrTwoColumn, attrCollapsible, attrReadonly,
			attrFormat, attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"text": {
		ID: "text",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"boolean": {
		ID: "boolean",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"dropdown": {
		ID: "dropdown",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"multioption": {
		ID: "multioption",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"radio": {
		ID: "radio",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"textarea": {
		ID: "textarea",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"number": {
		ID: "number",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"range": {
		ID: "range",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"date": {
		ID: "date",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"list": {
		ID: "list",
		ForbiddenAttributes: []string{
			attrSummaryField, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"table": {
		ID: "table",
		ForbiddenAttributes: []string{
			attrSummaryField, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"image": {
		ID: "image",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"link": {
		ID: "link",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"tags": {
		ID: "tags",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrCodeGroup, attrLatexGroup, attrAPIGroup,
		},
	},
	"code": {
		ID: "code",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrExpressionItem, attrTwoColumn,
			attrLatexGroup, attrAPIGroup,
		},
	},
	"latex": {
		ID: "latex",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrExpressionItem, attrTwoColumn,
			attrCodeGroup, attrAPIGroup,
		},
	},
	"api": {
		ID: "api",
		ForbiddenAttributes: []string{
			attrSummaryField, attrDefault, attrOptions, attrFormat,
			attrExpressionItem, attrTwoColumn, attrCollapsible, attrReadonly,
			attrCodeGroup, attrLatexGroup,
		},
	},
}

// IsKnownFieldType reports whether the given type id is in the registry.
func IsKnownFieldType(t string) bool {
	_, ok := fieldTypeRegistry[t]
	return ok
}

// orderedTypes is the public-facing iteration order of the registry —
// stable across calls so the frontend's "Type" dropdown lists types
// in a predictable order. Mirrors the original JS map declaration
// order so existing user habits don't shuffle.
var orderedTypes = []string{
	"text", "boolean", "dropdown", "multioption", "radio",
	"textarea", "number", "range", "date",
	"list", "table", "image", "link", "tags",
	"latex", "code", "api", "guid",
	"looper", "loopstart", "loopstop",
}

// AllFieldTypes returns the registry as a slice in the stable order
// declared by `orderedTypes`. Returned slices are defensive copies
// so callers can mutate without disturbing the registry. Used by
// Service.FieldTypes (the Wails-facing single source of truth).
func AllFieldTypes() []FieldTypeDef {
	out := make([]FieldTypeDef, 0, len(orderedTypes))
	for _, id := range orderedTypes {
		def, ok := fieldTypeRegistry[id]
		if !ok {
			continue
		}
		forbidden := append([]string(nil), def.ForbiddenAttributes...)
		out = append(out, FieldTypeDef{
			ID:                  def.ID,
			MetaOnly:            def.MetaOnly,
			ForbiddenAttributes: forbidden,
		})
	}
	return out
}

// propertyIsSet reports whether `attr` has a non-zero value on f.
// Group names ("code", "latex", "api") return true if any member of
// the group is set.
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
		return f.Default != nil
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
	case attrCodeGroup:
		return f.RunMode != "" || f.AllowRun != nil || f.HideField != nil ||
			f.InputMode != "" || f.APIMode != "" || len(f.APIPick) > 0
	case attrLatexGroup:
		return f.Rows != nil || f.UseFenced != nil
	case attrAPIGroup:
		return f.Collection != "" || f.ID != "" || len(f.Map) > 0 ||
			f.UsePicker != nil || len(f.AllowedIDs) > 0
	}
	return false
}
