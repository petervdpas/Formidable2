package csv

// Service is the api layer over Manager. Methods map to the old Electron
// `window.api.csv.*` IPC group:
//   - csv-preview     → Preview
//   - csv-write       → Write
//
// `csv-import-row` is intentionally NOT in this module — the Electron
// app already routed it to formManager.saveForm. Storage import (F-302)
// will own that route.
//
// Wails-only: HTTP export endpoints (Epic 8 collections API) call into
// this manager directly; no `handlers.go` lives here.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

func (s *Service) Preview(filePath, delimiter string) (PreviewResult, error) {
	return s.m.Preview(filePath, delimiter)
}

func (s *Service) Write(filePath string, rows [][]string, delimiter string) WriteResult {
	return s.m.Write(filePath, rows, delimiter)
}

// ApplyTransform runs one transform rule against a single cell value.
// mode is "preview" or "storage"; only meaningful for split-table.
func (s *Service) ApplyTransform(value, rule, param, mode string) string {
	m := ModeStorage
	if mode == "preview" {
		m = ModePreview
	}
	return Apply(value, rule, param, m)
}

// CoerceValue returns the typed value Coerce would produce for a CSV
// cell about to be written into a form field.
func (s *Service) CoerceValue(raw, fieldType string, options []any) any {
	return Coerce(raw, fieldType, options)
}

// CoercePreview returns a display-shaped string for the typed value.
func (s *Service) CoercePreview(raw, fieldType string, options []any) string {
	return CoercePreview(raw, fieldType, options)
}

// CoerceTableRows coerces a 2D string array (from a paste-data dialog
// on a table field) into a 2D typed array matching the column specs.
// Reuses Coerce per cell; dropdown columns match by value or label.
func (s *Service) CoerceTableRows(cols []TableColumn, rows [][]string) [][]any {
	return CoerceTableRows(cols, rows)
}

// SuggestMappings is the auto-mapper for the import dialog. Empty
// FieldKey in a row means "no match found".
func (s *Service) SuggestMappings(headers []string, fields []FieldSpec) []SuggestedMapping {
	return SuggestMappings(headers, fields)
}

// MappableFields filters out field types that cannot participate in
// a CSV mapping (image, code, api, loopstart/loopstop).
func (s *Service) MappableFields(fields []FieldSpec) []FieldSpec {
	return MappableFields(fields)
}

// TransformRules returns the ordered list of supported rule keys for
// the transform-selector dropdown. Frontend resolves labels via i18n.
func (s *Service) TransformRules() []string {
	return Rules()
}

// ExcludedFieldTypes returns the set of field types the import dialog
// must hide from the target-field dropdown.
func (s *Service) ExcludedFieldTypes() []string {
	return ExcludedFieldTypes()
}

// FormatValue is the export-side counterpart to CoerceValue: turn a
// stored typed value back into a CSV-friendly string.
func (s *Service) FormatValue(val any, fieldType string) string {
	return FormatValue(val, fieldType)
}

// Export is the one-call export pipeline: list forms, load each, build
// the row grid. The frontend then hands Rows to Write(filePath, ...).
func (s *Service) Export(templateFilename string, plan ExportPlan, fields []FieldSpec) ExportResult {
	return s.m.Export(templateFilename, plan, fields)
}

// BuildPreviewRows is the export-dialog's live preview helper. It runs
// the same row-building pipeline as Export but on caller-supplied
// entries (typically one) — no storage round trip. Always includes the
// header row at index 0.
func (s *Service) BuildPreviewRows(plan ExportPlan, entries []map[string]any, fields []FieldSpec) [][]string {
	return BuildExportRows(plan, entries, fields)
}
