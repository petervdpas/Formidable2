// Package csv reads and writes CSV files for Formidable2. Mirrors
// `controls/csvManager.js` semantics — header row first, RFC-4180
// quoting, configurable delimiter, LF line endings.
//
// Wails-only: the CSV import/export endpoints in Epic 8 (collections)
// will use this module from their HTTP handlers but no routes are
// registered directly by the csv module itself.
package csv

// PreviewResult is the parsed shape returned by Preview. Mirrors the JS
// `{headers, rows, rowCount, error}` so frontend handlers don't need
// to branch on shape.
type PreviewResult struct {
	Headers  []string   `json:"headers"`
	Rows     [][]string `json:"rows"`
	RowCount int        `json:"rowCount"`
	Error    string     `json:"error,omitempty"`
}

// WriteResult mirrors the JS shape.
type WriteResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
