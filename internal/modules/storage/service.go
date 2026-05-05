package storage

// Service is the api layer over Manager. Mirrors the Electron
// `window.api.forms.*` IPC group:
//   - forms-ensure-dir       → EnsureFormDir
//   - list-forms             → ListForms
//   - extended-list-forms    → ExtendedListForms
//   - load-form              → LoadForm
//   - save-form              → SaveForm
//   - delete-form            → DeleteForm
//   - save-image-file        → SaveImageFile
//   - csv-import-row         → ImportCsvRow (alias for SaveForm with raw envelope)
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

func (s *Service) EnsureFormDir(templateFilename string) error {
	return s.m.EnsureFormDir(templateFilename)
}

func (s *Service) ListForms(templateFilename string) ([]string, error) {
	return s.m.ListForms(templateFilename)
}

func (s *Service) ExtendedListForms(templateFilename string) ([]FormSummary, error) {
	return s.m.ExtendedListForms(templateFilename)
}

func (s *Service) LoadForm(templateFilename, datafile string) *Form {
	return s.m.LoadForm(templateFilename, datafile)
}

func (s *Service) SaveForm(templateFilename, datafile string, data map[string]any) SaveResult {
	return s.m.SaveForm(templateFilename, datafile, data)
}

func (s *Service) DeleteForm(templateFilename, datafile string) error {
	return s.m.DeleteForm(templateFilename, datafile)
}

func (s *Service) SaveImageFile(templateFilename, name string, content []byte) SaveResult {
	return s.m.SaveImageFile(templateFilename, name, content)
}

// LoadImageFile returns the named image as a base64 data URL.
func (s *Service) LoadImageFile(templateFilename, name string) (string, error) {
	return s.m.LoadImageFile(templateFilename, name)
}

// DeleteImageFile removes the named image from this template's images
// folder. Missing file is a no-op.
func (s *Service) DeleteImageFile(templateFilename, name string) error {
	return s.m.DeleteImageFile(templateFilename, name)
}

// ImportCsvRow is the storage-side of the old `csv-import-row` IPC.
// The frontend pre-parsed CSV and now wants each row stored as a form.
func (s *Service) ImportCsvRow(templateFilename, datafile string, data map[string]any) SaveResult {
	return s.m.SaveForm(templateFilename, datafile, data)
}
