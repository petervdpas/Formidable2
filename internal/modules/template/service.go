package template

// Service is the api layer over Manager. Method names mirror old
// `window.api.templates.*` IPC group (camelCase'd → PascalCase).
//
// Composition note: the storageLocation passed to GetDescriptor is
// supplied by the composition root, which knows about config's VFS.
// Keeps this module independent of config.
type Service struct {
	m              *Manager
	storageLocator func(templateFilename string) string
}

// NewService accepts a storage-locator function so the descriptor can
// include the storage location without this module depending on config.
// nil locator → empty StorageLocation.
func NewService(m *Manager, storageLocator func(string) string) *Service {
	if storageLocator == nil {
		storageLocator = func(string) string { return "" }
	}
	return &Service{m: m, storageLocator: storageLocator}
}

// TemplatesDir returns the absolute path of the templates folder.
// Used by the Utilities menu's "Open Template Folder" action; the
// frontend pipes the result through System.OpenExternal.
func (s *Service) TemplatesDir() string                        { return s.m.TemplatesDir() }
func (s *Service) ListTemplates() ([]string, error)            { return s.m.ListTemplates() }
func (s *Service) HasTemplates() bool                          { return s.m.HasTemplates() }
func (s *Service) LoadTemplate(name string) (*Template, error) { return s.m.LoadTemplate(name) }
func (s *Service) SaveTemplate(name string, t *Template) error { return s.m.SaveTemplate(name, t) }
func (s *Service) DeleteTemplate(name string) error            { return s.m.DeleteTemplate(name) }
// ValidateTemplate mirrors what SaveTemplate would see: a clone of
// the input is Normalized first, then Validated. That way the FE
// pre-save check returns exactly the errors a real save would, and
// disabled-attribute leftovers (e.g. legacy guid fields carrying
// label="GUID" / primary_key=true) self-heal silently rather than
// blocking the save.
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
func (s *Service) GetTemplateDescriptor(name string) (Descriptor, error) {
	return s.m.GetDescriptor(name, s.storageLocator(name))
}
func (s *Service) GetItemFields(name string) ([]ItemField, error) { return s.m.GetItemFields(name) }
func (s *Service) SeedBasicIfEmpty() error                        { return s.m.SeedBasicIfEmpty() }
func (s *Service) EnsureTemplateDirectory() error                 { return s.m.EnsureTemplateDirectory() }

// FieldTypes returns the registry of known field types and their
// forbidden attribute lists. The frontend uses this as the single
// source of truth for the "Type" dropdown and for editor-row
// visibility, so adding/changing a type happens in one place
// (field_registry.go).
func (s *Service) FieldTypes() []FieldDescriptor { return AllFieldTypes() }

// GeneratorShapes returns the catalog the frontend uses to populate
// the "Generate Template" dialog's shape picker.
func (s *Service) GeneratorShapes() []ShapeInfo { return Shapes() }

// GenerateMarkdown produces a default markdown_template body for the
// given fields in the chosen shape, with per-shape sub-options. Empty/
// unknown shape falls back to "report"; empty/unknown image mode falls
// back to "url".
//
// The fields argument comes from the unsaved Vue draft, so callers
// don't need to save before generating.
func (s *Service) GenerateMarkdown(shape string, opts GeneratorOptions, fields []Field) string {
	return GenerateMarkdownTemplate(Shape(shape), opts, fields)
}
