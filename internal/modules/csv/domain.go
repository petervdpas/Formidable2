package csv

import (
	stdcsv "encoding/csv"
	"fmt"
	"log/slog"
	"strings"
)

// fs is the filesystem surface this module needs.
type fs interface {
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
}

// formsSource is the storage surface the export side needs. Optional: callers
// that only use Preview/Write/transforms can leave it nil.
type formsSource interface {
	ListForms(tpl string) ([]string, error)
	LoadFormData(tpl, datafile string) map[string]any
}

// templateSource hands the export side a template's field schema so the backend
// owns excluded types, alignability, and the dotted "table.column" key.
type templateSource interface {
	Fields(tpl string) ([]FieldSpec, error)
}

const defaultDelimiter = ","

// Manager wraps encoding/csv with Formidable's preview/write conventions.
type Manager struct {
	fs    fs
	forms formsSource
	tpl   templateSource
	log   *slog.Logger
}

// NewManager constructs a CSV manager. log may be nil. The forms dependency
// is installed via SetForms after construction because the storage manager is
// built later in the composition root.
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

// SetTemplate installs the template dependency. ExportSchema/Export
// return a "template unavailable" error if this was never called.
func (m *Manager) SetTemplate(t templateSource) {
	m.tpl = t
}

// Preview reads filePath as CSV and returns its header row plus the rest.
// Empty delimiter falls back to comma. Errors surface both via the returned
// error and via PreviewResult.Error (the frontend reads the result object);
// Go callers should prefer the error.
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

// Write serializes rows (first row = headers, then data) to filePath. Empty
// delimiter falls back to comma; output uses LF line endings. When quoteAll is
// true every field is quoted (not just those RFC 4180 requires) so the file
// parses unambiguously in Excel regardless of the locale's delimiter. When
// false, encoding/csv applies minimal RFC-4180 quoting (only fields containing
// the delimiter, a quote, or a newline). Embedded quotes are escaped by
// doubling either way.
func (m *Manager) Write(filePath string, rows [][]string, delimiter string, quoteAll bool) WriteResult {
	delim := pickDelimiter(delimiter)
	var out strings.Builder
	if quoteAll {
		for _, row := range rows {
			for i, field := range row {
				if i > 0 {
					out.WriteRune(delim)
				}
				out.WriteByte('"')
				out.WriteString(strings.ReplaceAll(field, `"`, `""`))
				out.WriteByte('"')
			}
			out.WriteByte('\n')
		}
	} else {
		w := stdcsv.NewWriter(&out)
		w.Comma = delim
		w.WriteAll(rows)
		if err := w.Error(); err != nil {
			return WriteResult{Success: false, Error: err.Error()}
		}
	}
	if err := m.fs.SaveFile(filePath, out.String()); err != nil {
		return WriteResult{Success: false, Error: err.Error()}
	}
	return WriteResult{Success: true}
}

// pickDelimiter resolves an empty delimiter to the default and returns the
// first rune (encoding/csv only supports single-rune delimiters).
func pickDelimiter(d string) rune {
	if d == "" {
		d = defaultDelimiter
	}
	for _, r := range d {
		return r
	}
	return ','
}
