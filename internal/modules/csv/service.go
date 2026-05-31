package csv

// Service is the Wails surface over Manager. HTTP export endpoints (Epic 8
// collections API) call the manager directly, so no handlers.go lives here.
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

// CoerceTableRows coerces a 2D string array (from a paste-data dialog on a
// table field) into a 2D typed array matching the column specs. Dropdown
// columns match by value or label.
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

// MappableFieldsForTemplate returns the template's CSV-mappable field specs,
// excluded types stripped backend-side.
func (s *Service) MappableFieldsForTemplate(templateFilename string) ([]FieldSpec, error) {
	return s.m.MappableFieldsForTemplate(templateFilename)
}

// ExportSchema returns the default column plan, alignable fields, and source
// options for an alignment choice, derived from the template's field schema.
func (s *Service) ExportSchema(templateFilename, alignSource string) ExportSchema {
	return s.m.ExportSchema(templateFilename, alignSource)
}

// Export resolves fields, lists forms, loads each, and builds the row grid.
// The frontend then hands Rows to Write.
func (s *Service) Export(templateFilename string, plan ExportPlan) ExportResult {
	return s.m.Export(templateFilename, plan)
}

// PreviewExport returns the single data row the dialog shows under each
// column, built from the template's first stored form.
func (s *Service) PreviewExport(templateFilename string, plan ExportPlan) PreviewRowResult {
	return s.m.PreviewExport(templateFilename, plan)
}

// BuildImportForms reconstructs entries from parsed CSV rows, the inverse of
// Export. An aligned plan regroups the export's multiplied rows back into one
// entry per group with its nested list/table rebuilt.
func (s *Service) BuildImportForms(plan ImportPlan, headers []string, rows [][]string, fields []FieldSpec) []ImportForm {
	return BuildImportForms(plan, headers, rows, fields)
}
