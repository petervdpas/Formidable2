package template

// Service is the Wails layer over Manager. The storage-locator keeps this module independent of config.
type Service struct {
	m              *Manager
	storageLocator func(templateFilename string) string
}

// NewService takes a storage-locator so the descriptor carries the storage location without a config dep (nil -> "").
func NewService(m *Manager, storageLocator func(string) string) *Service {
	if storageLocator == nil {
		storageLocator = func(string) string { return "" }
	}
	return &Service{m: m, storageLocator: storageLocator}
}

// FacetMeta returns the full facet contract (max counts, palettes, validation patterns).
func (s *Service) FacetMeta() FacetMeta { return GetFacetMeta() }

// FormulaTargetTypes returns the formula-result-type -> acceptable target field
// types map, so the editor can scope a formula field's target picker.
func (s *Service) FormulaTargetTypes() map[string][]string { return FormulaTargetTypes() }

// TemplatesDir returns the absolute path of the templates folder.
func (s *Service) TemplatesDir() string                        { return s.m.TemplatesDir() }
func (s *Service) ListTemplates() ([]string, error)            { return s.m.ListTemplates() }
func (s *Service) HasTemplates() bool                          { return s.m.HasTemplates() }
func (s *Service) LoadTemplate(name string) (*Template, error) { return s.m.LoadTemplate(name) }

// LoadMany resolves a batch in one IPC call; per-row Error lets the rest of the batch render on a single failure.
func (s *Service) LoadMany(names []string) []LoadManyResult    { return s.m.LoadMany(names) }
func (s *Service) SaveTemplate(name string, t *Template) error { return s.m.SaveTemplate(name, t) }
func (s *Service) DeleteTemplate(name string) error            { return s.m.DeleteTemplate(name) }

// ValidateTemplate Normalizes a clone before validating so the FE pre-save check matches a real save
// and disabled-attribute leftovers self-heal rather than blocking.
func (s *Service) ValidateTemplate(t *Template) []ValidationError {
	if t == nil {
		return s.m.Validate(t)
	}
	clone := *t
	if t.Fields != nil {
		clone.Fields = append([]Field(nil), t.Fields...)
	}
	Normalize(&clone)
	return s.m.Validate(&clone)
}
// ValidateField returns only the validation errors a candidate field would
// introduce into the template (duplicate/missing keys, bindings, type/level
// rules), so the editor can gate its Confirm button on the backend instead of
// duplicating rules. originalKey + isNew locate the field (replace vs append).
func (s *Service) ValidateField(t *Template, field *Field, originalKey string, isNew bool) []ValidationError {
	if field == nil {
		return nil
	}
	return ValidateFieldDraft(t, *field, originalKey, isNew)
}

func (s *Service) GetTemplateDescriptor(name string) (Descriptor, error) {
	return s.m.GetDescriptor(name, s.storageLocator(name))
}
func (s *Service) GetItemFields(name string) ([]ItemField, error) { return s.m.GetItemFields(name) }
func (s *Service) SeedBasicIfEmpty() error                        { return s.m.SeedBasicIfEmpty() }
func (s *Service) EnsureTemplateDirectory() error                 { return s.m.EnsureTemplateDirectory() }

// FieldTypes returns the known field types and their forbidden attribute lists (source of truth: field_registry.go).
func (s *Service) FieldTypes() []FieldDescriptor { return AllFieldTypes() }

// TableColumnTypes returns the canonical column-type vocabulary; the frontend must not duplicate it.
func (s *Service) TableColumnTypes() []TableColumnTypeDescriptor {
	out := make([]TableColumnTypeDescriptor, len(builtinTableColumnTypes))
	copy(out, builtinTableColumnTypes)
	return out
}

// ListItemTypes returns the canonical item-type vocabulary; the frontend must not duplicate it.
func (s *Service) ListItemTypes() []ListItemTypeDescriptor {
	out := make([]ListItemTypeDescriptor, len(builtinListItemTypes))
	copy(out, builtinListItemTypes)
	return out
}

// GeneratorShapes returns the catalog for the "Generate Template" dialog's shape picker.
func (s *Service) GeneratorShapes() []ShapeInfo { return Shapes() }

// GenerateMarkdown produces a default markdown_template body from the unsaved draft fields.
func (s *Service) GenerateMarkdown(shape string, opts GeneratorOptions, fields []Field) string {
	return GenerateMarkdownTemplate(Shape(shape), opts, fields)
}

// BuildFieldTree groups a flat field list into the editor tree where each loopstart/loopstop pair is one FieldUnit.
func (s *Service) BuildFieldTree(fields []Field) []FieldUnit {
	return BuildFieldTree(fields)
}

// FlattenFieldTree is the inverse of BuildFieldTree, producing a flat field list for SaveTemplate.
func (s *Service) FlattenFieldTree(units []FieldUnit) []Field {
	return FlattenFieldTree(units)
}

// SummaryFieldCandidates lists the loop's direct child fields as Summary-picker options.
func (s *Service) SummaryFieldCandidates(fields []Field, loopKey string) []SummaryFieldOption {
	return SummaryFieldCandidates(fields, loopKey)
}
