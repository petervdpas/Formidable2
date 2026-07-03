package template

// Options-editor metadata: the backend-owned description of how a field type's options array (or a
// table/list sub-row) is edited. All *Key fields are i18n keys, not literal text. The frontend
// OptionsEditor renders these and hardcodes no shapes locally.

// SubRowEntry is one fixed slot inside a SubRow; Value is locked (e.g. "true"/"false") so only the label is editable.
type SubRowEntry struct {
	LabelKey       string `json:"label_key"`
	Value          string `json:"value"`
	PlaceholderKey string `json:"placeholder_key,omitempty"`
}

// SubRow declares an editor row shown below the main option row when its trigger column has this value.
// Input is stored pipe-delimited at row[RowKey] so parseChoices works unchanged (pair mode). When Scalar
// is set the row stores a single raw value at row[RowKey] instead (e.g. a number column's "step"); Default
// is the placeholder/fallback shown when the cell is empty. Entries set means fixed arity; nil means free-form.
type SubRow struct {
	RowKey         string        `json:"row_key"`
	LabelKey       string        `json:"label_key,omitempty"`
	PlaceholderKey string        `json:"placeholder_key,omitempty"`
	MaxEntries     int           `json:"max_entries,omitempty"`
	Entries        []SubRowEntry `json:"entries,omitempty"`
	Scalar         bool          `json:"scalar,omitempty"`
	Default        string        `json:"default,omitempty"`
}

// FixedOptionRow is one structurally fixed slot in a field's options array; Defaults fill cells short of the arity.
// Input overrides how the row's editable (label) cell renders in the editor:
// "format" (a preset dropdown), "color" (a colour picker), "number", else text.
type FixedOptionRow struct {
	LabelKey string         `json:"label_key"`
	Defaults map[string]any `json:"defaults"`
	Input    string         `json:"input,omitempty"`
}

// FixedOptionsShape declares an options array's fixed arity; nil/empty Rows means free-form.
// LockedColumns are rendered read-only across every row (e.g. the structural "value" key).
type FixedOptionsShape struct {
	Rows          []FixedOptionRow `json:"rows"`
	LockedColumns []string         `json:"locked_columns,omitempty"`
}
