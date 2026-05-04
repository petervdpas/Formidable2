package csv

import (
	stdcsv "encoding/csv"
	"fmt"
	"log/slog"
	"strings"
)

// fs is the narrow filesystem surface this module needs.
// *system.Manager satisfies it.
type fs interface {
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
}

const defaultDelimiter = ","

// Manager wraps encoding/csv with Formidable's preview/write conventions.
// Stateless beyond its dependencies.
type Manager struct {
	fs  fs
	log *slog.Logger
}

// NewManager constructs a CSV manager. log may be nil.
func NewManager(filesystem fs, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{fs: filesystem, log: log}
}

// Preview reads filePath as a CSV and returns its header row plus the
// rest of the rows. Empty delimiter falls back to comma.
//
// Errors are reported BOTH via the returned error AND via PreviewResult.Error
// so frontend code matching the JS shape (success-or-error in the result
// object) keeps working. Go callers should prefer the error.
func (m *Manager) Preview(filePath, delimiter string) (PreviewResult, error) {
	delim := pickDelimiter(delimiter)
	raw, err := m.fs.LoadFile(filePath)
	if err != nil {
		return PreviewResult{Headers: []string{}, Rows: [][]string{}, Error: err.Error()},
			fmt.Errorf("csv: read %q: %w", filePath, err)
	}
	if strings.TrimSpace(raw) == "" {
		return PreviewResult{Headers: []string{}, Rows: [][]string{}}, nil
	}

	r := stdcsv.NewReader(strings.NewReader(raw))
	r.Comma = delim
	r.LazyQuotes = false
	r.FieldsPerRecord = -1 // tolerate ragged rows; per-row length is the caller's problem

	records, err := r.ReadAll()
	if err != nil {
		return PreviewResult{Headers: []string{}, Rows: [][]string{}, Error: err.Error()},
			fmt.Errorf("csv: parse %q: %w", filePath, err)
	}
	if len(records) == 0 {
		return PreviewResult{Headers: []string{}, Rows: [][]string{}}, nil
	}

	headers := records[0]
	rows := records[1:]
	if rows == nil {
		rows = [][]string{}
	}
	return PreviewResult{
		Headers:  headers,
		Rows:     rows,
		RowCount: len(rows),
	}, nil
}

// Write serializes rows (first row = headers, then data) to filePath.
// Empty delimiter falls back to comma. Output uses LF line endings (not CRLF)
// to match the original JS implementation.
func (m *Manager) Write(filePath string, rows [][]string, delimiter string) WriteResult {
	delim := pickDelimiter(delimiter)
	var out strings.Builder
	if len(rows) > 0 {
		w := stdcsv.NewWriter(&out)
		w.Comma = delim
		w.UseCRLF = false
		if err := w.WriteAll(rows); err != nil {
			return WriteResult{Success: false, Error: err.Error()}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return WriteResult{Success: false, Error: err.Error()}
		}
	}
	if err := m.fs.SaveFile(filePath, out.String()); err != nil {
		return WriteResult{Success: false, Error: err.Error()}
	}
	return WriteResult{Success: true}
}

// pickDelimiter resolves an empty delimiter to the default and returns
// the first rune. Multi-rune inputs are truncated to their first rune
// (matches the encoding/csv API which only supports single-rune delimiters).
func pickDelimiter(d string) rune {
	if d == "" {
		d = defaultDelimiter
	}
	for _, r := range d {
		return r
	}
	return ','
}
