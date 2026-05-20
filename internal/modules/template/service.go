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

// FacetMeta returns the full facet contract (max counts, color +
// icon palettes, validation patterns). The frontend reads this once
// at boot so it doesn't mirror ANY of these constants — backend is
// the single source of truth.
func (s *Service) FacetMeta() FacetMeta { return GetFacetMeta() }

// TemplatesDir returns the absolute path of the templates folder.
// Used by the Utilities menu's "Open Template Folder" action; the
// frontend pipes the result through System.OpenExternal.
func (s *Service) TemplatesDir() string                        { return s.m.TemplatesDir() }
func (s *Service) ListTemplates() ([]string, error)            { return s.m.ListTemplates() }
func (s *Service) HasTemplates() bool                          { return s.m.HasTemplates() }
func (s *Service) LoadTemplate(name string) (*Template, error) { return s.m.LoadTemplate(name) }

// LoadMany resolves a batch of templates in one IPC call. Used by
// list workspaces to collapse N parallel LoadTemplate calls into
// one. Results carry per-row Error when a single file fails so the
// rest of the batch still renders.
func (s *Service) LoadMany(names []string) []LoadManyResult { return s.m.LoadMany(names) }
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

// TableColumnTypes returns the canonical column-type vocabulary the
// Edit Field modal's `table` preset surfaces. Single source of truth
// — the frontend MUST NOT keep a parallel hardcoded list.
func (s *Service) TableColumnTypes() []TableColumnTypeDescriptor {
	out := make([]TableColumnTypeDescriptor, len(builtinTableColumnTypes))
	copy(out, builtinTableColumnTypes)
	return out
}

// ListItemTypes returns the canonical item-type vocabulary the Edit
// Field modal's `list` preset surfaces. Same one-source-of-truth
// rule as TableColumnTypes — frontend reads via this method, never
// hardcodes the list.
func (s *Service) ListItemTypes() []ListItemTypeDescriptor {
	out := make([]ListItemTypeDescriptor, len(builtinListItemTypes))
	copy(out, builtinListItemTypes)
	return out
}

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

// BuildFieldTree groups a flat field list into the editor-facing tree
// where each matched loopstart/loopstop pair becomes one indivisible
// FieldUnit. The Vue editor renders this tree with nested draggables
// so a row cannot be reordered across a loop boundary by mistake.
func (s *Service) BuildFieldTree(fields []Field) []FieldUnit {
	return BuildFieldTree(fields)
}

// FlattenFieldTree returns the inverse: a tree the editor reordered
// becomes a well-formed flat field list ready for SaveTemplate. By
// construction loopstart/loopstop always bracket their items.
func (s *Service) FlattenFieldTree(units []FieldUnit) []Field {
	return FlattenFieldTree(units)
}
