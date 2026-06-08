package form

import "github.com/petervdpas/formidable2/internal/modules/storage"

// Service is the Wails-bound facade. Its surface is the single
// entry point Vue uses for form ops; template/storage services stay
// available for list/template-side work.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// BuildView returns the prepared FormView for a (template, datafile)
// pair. datafile == "" produces an unsaved view with defaults.
func (s *Service) BuildView(templateName, datafile string) (*FormView, error) {
	return s.m.BuildView(templateName, datafile)
}

// SaveValues persists the form (storage.Sanitize handles the heavy
// lifting; this layer adds author injection from config, then
// re-builds the view from disk).
func (s *Service) SaveValues(templateName string, payload SavePayload) (*FormView, error) {
	return s.m.SaveValues(templateName, payload)
}

// DeleteForm removes the meta.json. Missing is a no-op.
func (s *Service) DeleteForm(templateName, datafile string) error {
	return s.m.DeleteForm(templateName, datafile)
}

// ImportRelationEdges is the relations pass of a multipass import: it writes the
// pairs' target ids onto the existing source records' api field, saving each so
// the reference-edge syncer mirrors them into the relation graph. See
// Manager.ImportRelationEdges.
func (s *Service) ImportRelationEdges(sourceTemplate, fieldKey string, pairs []EdgePair) (ImportRelationResult, error) {
	return s.m.ImportRelationEdges(sourceTemplate, fieldKey, pairs)
}

// SortFieldValue fetches a list/table field from the saved record, sorts it,
// and returns the sorted value (no persistence: the frontend applies it and
// the normal save writes it). column is the table column key (empty = first
// column); direction is "asc" (default) or "desc". Ignored column for lists.
func (s *Service) SortFieldValue(templateName, datafile, fieldKey, column, direction string) (any, error) {
	return s.m.SortFieldValue(templateName, datafile, fieldKey, column, direction)
}

// DedupFieldValue fetches a list/table field from the saved record, removes
// duplicates, and returns the result (no persistence). column is the table
// column key whose value marks a duplicate row (empty = first column); ignored
// for list fields.
func (s *Service) DedupFieldValue(templateName, datafile, fieldKey, column string) (any, error) {
	return s.m.DedupFieldValue(templateName, datafile, fieldKey, column)
}

// ListForms returns the per-template form summaries (title + meta +
// expression-bearing fields for the sidebar).
func (s *Service) ListForms(templateName string) ([]storage.FormSummary, error) {
	return s.m.ListForms(templateName)
}

// EnsureFormDir creates the per-template storage folder. Vue calls
// this on first list against a freshly-created template.
func (s *Service) EnsureFormDir(templateName string) error {
	return s.m.EnsureFormDir(templateName)
}
