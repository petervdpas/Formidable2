package expression

// Result is the rendered output of one expression evaluation; Items is set when the expression returns a list.
type Result struct {
	Text    string   `json:"text"`
	Color   string   `json:"color,omitempty"`
	Bg      string   `json:"bg,omitempty"`
	Classes []string `json:"classes,omitempty"`
	Items   []string `json:"items,omitempty"`

	// Filename pairs a list-path result back to its record; empty for single-shot Evaluate.
	Filename string `json:"filename,omitempty"`

	// Error carries the per-row failure message; the Manager keeps going so one bad row doesn't blank the list.
	Error string `json:"error,omitempty"`
}
