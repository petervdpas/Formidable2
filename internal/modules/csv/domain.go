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

// formsSource is the narrow storage surface BuildExportRows needs.
// *storage.Manager satisfies it via a thin adapter wired in app.go.
// Optional: callers that only use Preview/Write/transforms can leave
// the manager's forms field nil.
type formsSource interface {
	ListForms(tpl string) ([]string, error)
	LoadFormData(tpl, datafile string) map[string]any
}

const defaultDelimiter = ","

// Manager wraps encoding/csv with Formidable's preview/write conventions.
// Stateless beyond its dependencies.
type Manager struct {
	fs    fs
	forms formsSource
	log   *slog.Logger
}

// NewManager constructs a CSV manager. log may be nil. The forms
// dependency is installed via SetForms after construction because the
// storage manager is built later in the composition root.
func NewManager(filesystem fs, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{fs: filesystem, log: log}
}

// SetForms installs the storage dependency. Export() returns a
// "storage unavailable" error if this was never called.
func (m *Manager) SetForms(f formsSource) {
	m.forms = f
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
