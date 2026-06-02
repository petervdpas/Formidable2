package mermaid

// Issue is one validation problem at a 1-based line/column. Code is the stable
// key the frontend translates; Severity is "error" | "warning" | "info";
// Message is a developer-facing fallback. Line/Col are 0 when not positioned.
type Issue struct {
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Result is the flat, wire-friendly outcome of validating Mermaid source.
// OK is false only when at least one error-severity issue is present.
type Result struct {
	OK          bool    `json:"ok"`
	DiagramType string  `json:"diagramType"`
	Errors      []Issue `json:"errors"`
}
