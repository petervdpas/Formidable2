package builder

// TextSourceOption is one option the OUTCOME text-part field-value picker
// offers: a key (the F["key"] it compiles to), a display label, and a group so
// the UI can separate real fields from formula fields. Assembled by the Service
// (it has the template) from the displayable fields plus the formula catalog,
// so the frontend renders the list rather than deciding what is selectable.
type TextSourceOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Group string `json:"group"` // "field" | "formula"
}
