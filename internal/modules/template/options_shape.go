package template

// Options-editor metadata: the canonical, backend-owned description of
// how a field type's `options` array - or a table/list column-type's
// triggered sub-row - should be edited. The frontend OptionsEditor
// reads these structures via TemplateSvc.FieldTypes /
// TableColumnTypes / ListItemTypes and renders them; it does NOT
// hardcode the shapes locally (see [[feedback_backend_owns_data]]).

// All user-facing strings on these descriptors are i18n KEYS, not
// literal text - the frontend resolves them via vue-i18n. Backend
// owns the structure; locale catalogs own the wording (see
// internal/modules/i18n/locales/<lang>/templates.json). Per
// [[feedback_i18n_central]].

// SubRowEntry is one fixed slot inside a SubRow. Each entry locks a
// canonical Value (e.g. "true" / "false" for a bool column) - the
// user only edits the human-readable label. LabelKey is the i18n
// key for the gutter caption shown next to the locked value.
type SubRowEntry struct {
	LabelKey       string `json:"label_key"`
	Value          string `json:"value"`
	PlaceholderKey string `json:"placeholder_key,omitempty"`
}

// SubRow declares an extra editor row that appears below the main
// option row when its triggering dropdown column's current value is
// this one. The user's input is stored as a single pipe-delimited
// string at row[RowKey] so the form renderer's parseChoices works
// unchanged. Either populate Entries (fixed-arity, one input per
// entry) OR leave it nil for a free-form add/remove pair editor.
// LabelKey + PlaceholderKey are i18n keys.
type SubRow struct {
	RowKey         string        `json:"row_key"`
	LabelKey       string        `json:"label_key,omitempty"`
	PlaceholderKey string        `json:"placeholder_key,omitempty"`
	MaxEntries     int           `json:"max_entries,omitempty"`
	Entries        []SubRowEntry `json:"entries,omitempty"`
}

// FixedOptionRow is one row in a FixedOptionsShape - a structurally
// fixed slot in a field's options array (e.g. True / False rows for
// a bool field). Defaults populate the cells when the user first
// picks the field type or when an existing options array arrives
// short of the configured arity. LabelKey is the i18n key for the
// row's gutter caption.
type FixedOptionRow struct {
	LabelKey string         `json:"label_key"`
	Defaults map[string]any `json:"defaults"`
}

// FixedOptionsShape declares the options array's fixed arity for a
// field type. nil/empty Rows = free-form (add/remove enabled).
// LockedColumns names the column keys the editor renders read-only
// across every row (e.g. the structural "value" key for boolean's
// true/false or range's min/max/step - only the label is editable).
type FixedOptionsShape struct {
	Rows          []FixedOptionRow `json:"rows"`
	LockedColumns []string         `json:"locked_columns,omitempty"`
}
