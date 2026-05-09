package expression

// SidebarItem is the rendered output of one expression evaluation.
// Frontends render Text as the sub-label and apply Color (CSS color)
// + Classes (utility CSS classes from styles/expression.css). Items
// is populated when the expression returns a list — frontends can
// render chips/pills instead of a single label if they want.
//
// All fields are JSON-encoded with omitempty so a bare `{Text:"x"}`
// payload stays compact across the Wails surface.
type SidebarItem struct {
	Text    string   `json:"text"`
	Color   string   `json:"color,omitempty"`
	Bg      string   `json:"bg,omitempty"`
	Classes []string `json:"classes,omitempty"`
	Items   []string `json:"items,omitempty"`

	// Filename is set by EvaluateSidebar so the frontend can pair the
	// rendered label with its record without a parallel array. Empty
	// for single-shot Evaluate calls — only meaningful for the bulk
	// sidebar path.
	Filename string `json:"filename,omitempty"`

	// Error carries the per-row failure message when evaluation fails
	// for one record. The Manager keeps going; the frontend can show
	// a tiny `[expr error]` chip without nuking the whole list.
	Error string `json:"error,omitempty"`
}
