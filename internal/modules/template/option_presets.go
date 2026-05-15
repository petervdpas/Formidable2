package template

// TableColumnTypeDescriptor names one column type the Edit Field
// modal's `table` preset offers in its column-type dropdown. Today
// these strings are pure UI vocabulary — the Go side does not yet
// validate table cell data against them — but the registry lives
// here so a future server-side validation pass has a single source
// of truth to read.
type TableColumnTypeDescriptor struct {
	Name string `json:"name"`
}

// ListItemTypeDescriptor names one entry the Edit Field modal's
// `list` preset offers in its item-type dropdown. Same rationale as
// TableColumnTypeDescriptor — captured here even though Go doesn't
// interpret the strings yet, so the future home is ready.
type ListItemTypeDescriptor struct {
	Name string `json:"name"`
}

// builtinTableColumnTypes is the canonical column-type set the table
// preset surfaces in its dropdown. Display order is significant — the
// frontend renders the dropdown in the order returned here.
//
// "reference" is reserved-but-deferred. At the data layer the cell is
// just a string — same as any text column. The "reference" type means
// the renderer treats that string as a potential link target: at
// render time, string-compare it against looper-entry codes (looper /
// loopstart / loopstop field types, see field_registry.go) and emit
// an HTML anchor (<a href="#...">) on match, the same way a TOC entry
// renders as a link to its heading. No match → plain text. That's
// where the NAME comes from: the type describes the RENDERED output.
//
// A picker UX on top of the cell is a convenience — populate a
// dropdown from the in-scope looper's code values so the user doesn't
// have to retype — but it's not load-bearing; the underlying value
// stays a string and free-form typing must still be allowed.
//
// What's deferred: the renderer's anchor-emit pass + the optional
// picker UX. The `option-presets.ts` comment "Subrows (choices /
// reference) for table are deferred" tracks the same gap on the
// frontend side.
//
// Keep in sync with the documented option-editor matrix. When new
// types ship, add them here and (if applicable) wire validation under
// internal/modules/template.
var builtinTableColumnTypes = []TableColumnTypeDescriptor{
	{Name: "string"},
	{Name: "number"},
	{Name: "date"},
	{Name: "bool"},
	{Name: "dropdown"},
	{Name: "reference"},
}

// builtinListItemTypes is the canonical item-type set the list preset
// surfaces. "fixed" = a literal value; "custom" = `[[custom]]` token
// that the list field renders as a free-text input. The frontend
// hooks an onChange to lock the value to [[custom]] when "custom" is
// picked — see frontend/src/types/option-presets.ts.
var builtinListItemTypes = []ListItemTypeDescriptor{
	{Name: "fixed"},
	{Name: "custom"},
}
