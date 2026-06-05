package template

// TableColumnTypeDescriptor names one column type the table preset's column-type dropdown offers.
type TableColumnTypeDescriptor struct {
	Name   string  `json:"name"`
	SubRow *SubRow `json:"sub_row,omitempty"`
}

// ListItemTypeDescriptor names one entry the list preset's item-type dropdown offers.
type ListItemTypeDescriptor struct {
	Name   string  `json:"name"`
	SubRow *SubRow `json:"sub_row,omitempty"`
}

// builtinTableColumnTypes is the canonical column-type set; display order is significant.
// "reference" is reserved-but-deferred: the cell stays a string, but the renderer is meant to
// string-compare it against looper-entry codes and emit an anchor on match (the deferred anchor-emit pass).
var builtinTableColumnTypes = []TableColumnTypeDescriptor{
	{Name: "string"},
	{
		Name: "number",
		SubRow: &SubRow{
			RowKey:         "step",
			LabelKey:       "workspace.templates.options.number_subrow",
			PlaceholderKey: "workspace.templates.options.number_step_placeholder",
			Scalar:         true,
			Default:        "1",
		},
	},
	{Name: "date"},
	{
		Name: "bool",
		SubRow: &SubRow{
			RowKey:   "choices",
			LabelKey: "workspace.templates.options.bool_subrow",
			// Two canonical states; the widget locks the Value column so only labels are editable.
			Entries: []SubRowEntry{
				{LabelKey: "common.true", Value: "true", PlaceholderKey: "workspace.templates.options.bool_placeholder_true"},
				{LabelKey: "common.false", Value: "false", PlaceholderKey: "workspace.templates.options.bool_placeholder_false"},
			},
			MaxEntries: 2,
		},
	},
	{
		Name: "dropdown",
		SubRow: &SubRow{
			RowKey:         "choices",
			LabelKey:       "workspace.templates.options.dropdown_subrow",
			PlaceholderKey: "workspace.templates.options.dropdown_placeholder",
		},
	},
	{Name: "reference"},
}

// builtinListItemTypes is the canonical item-type set: "fixed" is a literal; "custom" is the
// [[custom]] token the list field renders as a free-text input.
var builtinListItemTypes = []ListItemTypeDescriptor{
	{Name: "fixed"},
	{Name: "custom"},
}
