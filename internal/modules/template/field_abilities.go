package template

// Per-field-type ability matrix, fully dense by design so adding/removing an ability forces an audit here.
// ability=true means the modal row shows and the value is preserved on save.

var fieldDescriptors = map[string]FieldDescriptor{
	"text": {
		ID: "text",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: true, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"textarea": {
		ID: "textarea",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: true, Format: true, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"mermaid": {
		ID: "mermaid",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"number": {
		ID: "number",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"range": {
		ID: "range",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
		OptionsShape: &FixedOptionsShape{
			Rows: []FixedOptionRow{
				{LabelKey: "workspace.templates.field_edit.range.min", Defaults: map[string]any{"value": "min", "label": "0"}},
				{LabelKey: "workspace.templates.field_edit.range.max", Defaults: map[string]any{"value": "max", "label": "10"}},
				{LabelKey: "workspace.templates.field_edit.range.step", Defaults: map[string]any{"value": "step", "label": "1"}},
			},
			LockedColumns: []string{"value"},
		},
	},
	"date": {
		ID: "date",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"boolean": {
		ID: "boolean",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
		OptionsShape: &FixedOptionsShape{
			Rows: []FixedOptionRow{
				{LabelKey: "common.true", Defaults: map[string]any{"value": "true", "label": "Yes"}},
				{LabelKey: "common.false", Defaults: map[string]any{"value": "false", "label": "No"}},
			},
			LockedColumns: []string{"value"},
		},
	},
	"dropdown": {
		ID: "dropdown",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"multioption": {
		ID: "multioption",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"radio": {
		ID: "radio",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"file-path": {
		ID: "file-path",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: true, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"folder-path": {
		ID: "folder-path",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: true, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"list": {
		ID: "list",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: true,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"table": {
		ID: "table",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: true,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"image": {
		ID: "image",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"link": {
		ID: "link",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: true,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"tags": {
		ID: "tags",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: true,
			FacetKey: false,
		},
	},
	"api": {
		ID: "api",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: false, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"guid": {
		ID: "guid",
		Abilities: Abilities{
			Key: true, Type: true, Label: false, Description: false,
			Default: false, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"facet": {
		ID: "facet", Virtual: true,
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: true, UseInStatistics: false,
			FacetKey: true,
		},
	},
	"formula": {
		ID: "formula", Virtual: true,
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: false, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"looper": {
		ID: "looper", MetaOnly: true,
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: false, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"loopstart": {
		ID: "loopstart", MetaOnly: true,
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: false, Options: false, SummaryField: true, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
	"loopstop": {
		ID: "loopstop", MetaOnly: true,
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: false,
			Default: false, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false, UseInStatistics: false,
			FacetKey: false,
		},
	},
}

// orderedTypes is the stable iteration order so the frontend's "Type" dropdown is predictable.
var orderedTypes = []string{
	"text", "textarea", "mermaid", "number", "range", "date",
	"boolean", "dropdown", "multioption", "radio",
	"file-path", "folder-path",
	"list", "table", "image", "link", "tags",
	"api", "guid", "facet", "formula",
	"looper", "loopstart", "loopstop",
}

// fieldTypeLabelKeys is the i18n key for each type's display label. Literal keys
// (no interpolation) so they stay greppable, and the backend is the single
// source of truth - the frontend reads these off the descriptor, it does not
// keep its own copy. Every orderedTypes entry must have one (audited by test).
var fieldTypeLabelKeys = map[string]string{
	"text":        "workspace.templates.field_type.text",
	"textarea":    "workspace.templates.field_type.textarea",
	"mermaid":     "workspace.templates.field_type.mermaid",
	"number":      "workspace.templates.field_type.number",
	"range":       "workspace.templates.field_type.range",
	"date":        "workspace.templates.field_type.date",
	"boolean":     "workspace.templates.field_type.boolean",
	"dropdown":    "workspace.templates.field_type.dropdown",
	"multioption": "workspace.templates.field_type.multioption",
	"radio":       "workspace.templates.field_type.radio",
	"file-path":   "workspace.templates.field_type.file_path",
	"folder-path": "workspace.templates.field_type.folder_path",
	"list":        "workspace.templates.field_type.list",
	"table":       "workspace.templates.field_type.table",
	"image":       "workspace.templates.field_type.image",
	"link":        "workspace.templates.field_type.link",
	"tags":        "workspace.templates.field_type.tags",
	"api":         "workspace.templates.field_type.api",
	"guid":        "workspace.templates.field_type.guid",
	"facet":       "workspace.templates.field_type.facet",
	"formula":     "workspace.templates.field_type.formula",
	"looper":      "workspace.templates.field_type.looper",
	"loopstart":   "workspace.templates.field_type.loopstart",
	"loopstop":    "workspace.templates.field_type.loopstop",
}

// fieldTypeDefaults is the value used to seed a freshly-created field's Default
// in the editor. Backend-owned (the frontend wraps each in a clone-factory).
// Meta/virtual types are intentionally absent (nil default -> no seed).
var fieldTypeDefaults = map[string]any{
	"text":        "",
	"textarea":    "",
	"mermaid":     "",
	"number":      0,
	"range":       50,
	"date":        "",
	"boolean":     false,
	"dropdown":    "",
	"multioption": []any{},
	"radio":       "",
	"file-path":   "",
	"folder-path": "",
	"list":        []any{},
	"table":       []any{},
	"image":       "",
	"link":        map[string]any{"href": "", "text": ""},
	"tags":        []any{},
	"api":         map[string]any{"id": "", "overrides": map[string]any{}},
	"guid":        "",
}
