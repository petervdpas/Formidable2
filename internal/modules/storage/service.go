package storage

import (
	"context"

	"github.com/petervdpas/formidable2/internal/event"
)

// Service is the Wails layer over Manager.
type Service struct {
	m    *Manager
	emit event.Emitter
}

func NewService(m *Manager, emit event.Emitter) *Service { return &Service{m: m, emit: emit} }

func (s *Service) EnsureFormDir(templateFilename string) error {
	return s.m.EnsureFormDir(templateFilename)
}

// TemplateStorageDir returns the absolute path of this template's storage folder.
func (s *Service) TemplateStorageDir(templateFilename string) string {
	return s.m.TemplateStorageDir(templateFilename)
}

func (s *Service) ListForms(templateFilename string) ([]string, error) {
	return s.m.ListForms(templateFilename)
}

func (s *Service) ExtendedListForms(templateFilename string) ([]FormSummary, error) {
	return s.m.ExtendedListForms(templateFilename)
}

// SearchForms runs a full-text query, returning matching summaries ranked by relevance (empty query: no rows).
func (s *Service) SearchForms(templateFilename, query string) ([]FormSummary, error) {
	return s.m.SearchForms(templateFilename, query)
}

func (s *Service) LoadForm(templateFilename, datafile string) *Form {
	return s.m.LoadForm(templateFilename, datafile)
}

// SaveForm passes Background because Wails IPC has no request context; stamp() falls back to the AuthorProvider.
func (s *Service) SaveForm(templateFilename, datafile string, data map[string]any) SaveResult {
	return s.m.SaveForm(context.Background(), templateFilename, datafile, data)
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

// DeleteImageFile removes the named image (missing file is a no-op).
func (s *Service) DeleteImageFile(templateFilename, name string) error {
	return s.m.DeleteImageFile(templateFilename, name)
}

// ImageFileExists reports whether an image asset already exists (for name uniquifying).
func (s *Service) ImageFileExists(templateFilename, name string) bool {
	return s.m.ImageFileExists(templateFilename, name)
}

// SlugifyEntryName turns a freely-typed name into a valid datafile stem (the
// backend-owned filename rule). The frontend calls this instead of slugging
// client-side, so the rule lives in one place.
func (s *Service) SlugifyEntryName(raw string) string {
	return SlugifyDatafileStem(raw)
}

// ListImageFiles returns the template's image assets (the reusable library), sorted.
func (s *Service) ListImageFiles(templateFilename string) ([]string, error) {
	return s.m.ListImageFiles(templateFilename)
}

// RenameImageFile moves an image asset to a new name within the same template.
func (s *Service) RenameImageFile(templateFilename, oldName, newName string) error {
	return s.m.RenameImageFile(templateFilename, oldName, newName)
}

// RenameImageAcrossForms renames a library image and rewrites references to it
// across the template's forms, returning how many forms were updated.
func (s *Service) RenameImageAcrossForms(templateFilename, oldName, newName string) (int, error) {
	return s.m.RenameImageAcrossForms(context.Background(), templateFilename, oldName, newName)
}

// ImportCsvRow stores one pre-parsed CSV row as a form.
func (s *Service) ImportCsvRow(templateFilename, datafile string, data map[string]any) SaveResult {
	return s.m.SaveForm(context.Background(), templateFilename, datafile, data)
}

// MigrateTemplateMeta rewrites every legacy-shaped form under the template into the new audit-block shape.
// On a real rewrite it emits storage:changed so the frontend reloads the affected forms from disk.
func (s *Service) MigrateTemplateMeta(templateFilename string) (MigrateResult, error) {
	res, err := s.m.MigrateTemplateMeta(templateFilename)
	if err == nil && res.Migrated > 0 {
		event.Emit(s.emit, "storage:changed", templateFilename)
	}
	return res, err
}
