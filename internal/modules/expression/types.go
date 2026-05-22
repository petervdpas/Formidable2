package expression

// Result is the rendered output of one expression evaluation.
// Frontends render Text as the row label and apply Color (CSS color)
// + Classes (utility CSS classes from styles/expression.css). Items
// is populated when the expression returns a list - consumers can
// render chips/pills instead of a single label if they want.
//
// All fields are JSON-encoded with omitempty so a bare `{Text:"x"}`
// payload stays compact across the Wails surface.
type Result struct {
	Text    string   `json:"text"`
	Color   string   `json:"color,omitempty"`
	Bg      string   `json:"bg,omitempty"`
	Classes []string `json:"classes,omitempty"`
	Items   []string `json:"items,omitempty"`

	// Filename is set by EvaluateList / EvaluateListOne / EvaluateListMany
	// so callers can pair the rendered label with its record without a
	// parallel array. Empty for single-shot Evaluate calls - only
	// meaningful for the per-record list paths.
	Filename string `json:"filename,omitempty"`

	// Error carries the per-row failure message when evaluation fails
	// for one record. The Manager keeps going; consumers can show a
	// tiny `[expr error]` chip without nuking the whole list.
	Error string `json:"error,omitempty"`
}
