// Package csv reads and writes CSV files for Formidable: header row first,
// RFC-4180 quoting, configurable delimiter, LF line endings. The csv module
// registers no HTTP routes; Epic 8 collections endpoints call it directly.
package csv

// PreviewResult is the parsed shape returned by Preview.
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
