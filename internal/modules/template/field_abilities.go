package template

// Per-field-type ability matrix. Every type sets every Abilities bool
// explicitly — the matrix is fully dense by design so adding/removing
// an ability anywhere in the codebase forces an audit here.
//
// Translated from the original `utils/fieldTypes.js` `disabledAttributes`
// arrays (inverted: ability=true means the modal row is shown and the
// value is preserved on save). file-path / folder-path are Formidable2
// additions with no original counterpart; code/latex types from the
// original are intentionally not present (removed in Formidable2).

var fieldDescriptors = map[string]FieldDescriptor{
	"text": {
		ID: "text",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: true, Format: false,
		},
	},
	"textarea": {
		ID: "textarea",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: true, Format: true,
		},
	},
	"number": {
		ID: "number",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"range": {
		ID: "range",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"date": {
		ID: "date",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"boolean": {
		ID: "boolean",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"dropdown": {
		ID: "dropdown",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"multioption": {
		ID: "multioption",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"radio": {
		ID: "radio",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: true, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"file-path": {
		ID: "file-path",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: true, Format: false,
		},
	},
	"folder-path": {
		ID: "folder-path",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: true, Format: false,
		},
	},
	"list": {
		ID: "list",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: true,
			Readonly: false, Format: false,
		},
	},
	"table": {
		ID: "table",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: true, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: true,
			Readonly: false, Format: false,
		},
	},
	"image": {
		ID: "image",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"link": {
		ID: "link",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"tags": {
		ID: "tags",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: true, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: true, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"api": {
		ID: "api",
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: false, Options: false, SummaryField: false, PrimaryKey: true,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"guid": {
		ID: "guid",
		Abilities: Abilities{
			Key: true, Type: true, Label: false, Description: false,
			Default: false, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"looper": {
		ID: "looper", MetaOnly: true,
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: false, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"loopstart": {
		ID: "loopstart", MetaOnly: true,
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: true,
			Default: false, Options: false, SummaryField: true, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
	"loopstop": {
		ID: "loopstop", MetaOnly: true,
		Abilities: Abilities{
			Key: true, Type: true, Label: true, Description: false,
			Default: false, Options: false, SummaryField: false, PrimaryKey: false,
			ExpressionItem: false, TwoColumn: false, Collapsible: false,
			Readonly: false, Format: false,
		},
	},
}

// orderedTypes is the public-facing iteration order of the matrix —
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
