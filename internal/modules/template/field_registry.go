package template

// Field-type registry. Mirrors `utils/fieldTypes.js` from the original
// Formidable: each entry declares which optional Field properties are
// FORBIDDEN for that type. The frontend's `field-types.ts` registry
// drives editor visibility for the same set of properties; here we
// enforce the contract at save-time so a hand-edited YAML or a
// programmatic writer can't leave meaningless data on a field.

// Property names used across the registry. Stable strings — referenced
// by tests and by the ValidationError detail. The "api" group name
// checks the union of its members.
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
	attrAPIGroup       = "api" // collection / id / map / use_picker / allowed_ids
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
		// Allowed on guid: label, description, primary_key — a guid field
		// is the natural primary key of a collection-enabled template,
		// and the editor lets the user label it ("GUID") and describe it.
		ForbiddenAttributes: []string{
			attrDefault,
			attrOptions, attrSummaryField, attrExpressionItem, attrTwoColumn,
			attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"looper": {
		ID: "looper", MetaOnly: true,
		ForbiddenAttributes: []string{
			attrDefault, attrOptions, attrExpressionItem, attrTwoColumn,
			attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"loopstart": {
		ID: "loopstart", MetaOnly: true,
		ForbiddenAttributes: []string{
			attrDefault, attrOptions, attrExpressionItem, attrTwoColumn,
			attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"loopstop": {
		ID: "loopstop", MetaOnly: true,
		ForbiddenAttributes: []string{
			attrSummaryField, attrDescription, attrDefault, attrOptions,
			attrExpressionItem, attrTwoColumn, attrCollapsible, attrReadonly,
			attrFormat, attrAPIGroup,
		},
	},
	"text": {
		ID: "text",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrFormat,
			attrAPIGroup,
		},
	},
	"file-path": {
		// Path-shaped text input — a plain string value paired with a
		// Browse button (native file picker). Options carry extension
		// globs ("*.json", "*.md;*.markdown") that become FileFilter
		// entries in the picker dropdown.
		ID: "file-path",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrFormat,
			attrAPIGroup,
		},
	},
	"folder-path": {
		// As file-path but the picker accepts a directory. No filters
		// apply to a directory picker, so options are forbidden here.
		ID: "folder-path",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrFormat,
			attrAPIGroup, attrOptions,
		},
	},
	"boolean": {
		ID: "boolean",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"dropdown": {
		ID: "dropdown",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"multioption": {
		ID: "multioption",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"radio": {
		ID: "radio",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"textarea": {
		ID: "textarea",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible,
			attrAPIGroup,
		},
	},
	"number": {
		ID: "number",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"range": {
		ID: "range",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"date": {
		ID: "date",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"list": {
		ID: "list",
		ForbiddenAttributes: []string{
			attrSummaryField, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"table": {
		ID: "table",
		ForbiddenAttributes: []string{
			attrSummaryField, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"image": {
		ID: "image",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"link": {
		ID: "link",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"tags": {
		ID: "tags",
		ForbiddenAttributes: []string{
			attrSummaryField, attrCollapsible, attrReadonly, attrFormat,
			attrAPIGroup,
		},
	},
	"api": {
		ID: "api",
		ForbiddenAttributes: []string{
			attrSummaryField, attrDefault, attrOptions, attrFormat,
			attrExpressionItem, attrTwoColumn, attrCollapsible, attrReadonly,
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
	"text", "textarea", "number", "range", "date",
	"boolean", "dropdown", "multioption", "radio",
	"file-path", "folder-path",
	"list", "table", "image", "link", "tags",
	"api", "guid",
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

// defaultIsPopulated reports whether a Field.Default value should be
// treated as "explicitly set" by the user. Nil/empty-string/zero-number/
// false-bool are quiet zeros that arrive via YAML round-trip even when
// the user never typed a default — flagging them as forbidden creates
// false positives on guid/loopstart/loopstop seed fields.
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
// The "api" group name returns true if any member of the group is set.
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
		// YAML `default: ""` (or `default: 0` / `default: false`) is the
		// editor's quiet zero — not "default explicitly set". Only flag
		// truly-populated defaults so seed templates with empty defaults
		// don't trip the forbidden-attribute rule on guid/loop fields.
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
	case attrAPIGroup:
		return f.Collection != "" || len(f.Map) > 0
	}
	return false
}
