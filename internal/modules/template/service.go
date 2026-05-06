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

func (s *Service) ListTemplates() ([]string, error)            { return s.m.ListTemplates() }
func (s *Service) LoadTemplate(name string) (*Template, error) { return s.m.LoadTemplate(name) }
func (s *Service) SaveTemplate(name string, t *Template) error { return s.m.SaveTemplate(name, t) }
func (s *Service) DeleteTemplate(name string) error            { return s.m.DeleteTemplate(name) }
func (s *Service) ValidateTemplate(t *Template) []ValidationError {
	return s.m.Validate(t)
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
func (s *Service) FieldTypes() []FieldTypeDef { return AllFieldTypes() }
