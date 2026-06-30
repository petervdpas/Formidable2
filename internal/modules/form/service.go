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

// CopyForm duplicates sourceDatafile into newDatafile with a fresh identity (new
// GUID, new Created/Updated), keeping every field value, tag and facet. Returns
// the saved view of the new record, ready for the editor to open.
func (s *Service) CopyForm(templateName, sourceDatafile, newDatafile string) (*FormView, error) {
	return s.m.CopyForm(templateName, sourceDatafile, newDatafile)
}

// RelationFields returns the source template's api fields, the relation targets
// the relations-import mode can fill. The dialog's relation picker reads this
// instead of filtering the template client-side.
func (s *Service) RelationFields(sourceTemplate string) ([]RelationField, error) {
	return s.m.RelationFields(sourceTemplate)
}

// ImportRelations is the relations pass of a multipass import: given a parsed
// sheet (headers + rows) and the two id columns, the backend extracts the
// {from,to} pairs and links them through fieldKey, writing the api value onto
// each source record so the reference-edge syncer mirrors the edges. See
// Manager.ImportRelationsFromColumns.
func (s *Service) ImportRelations(sourceTemplate, fieldKey, fromColumn, toColumn string, headers []string, rows [][]string) (ImportRelationResult, error) {
	return s.m.ImportRelationsFromColumns(sourceTemplate, fieldKey, fromColumn, toColumn, headers, rows)
}

// SyncRelationsToField back-fills an api field from the relation edges that
// already exist for it (e.g. an inverse field added after the links were made),
// writing each host record's target ids so field, edges, and graph agree. See
// Manager.SyncRelationsToField. Idempotent.
func (s *Service) SyncRelationsToField(template, fieldKey string) (ImportRelationResult, error) {
	return s.m.SyncRelationsToField(template, fieldKey)
}

// SyncRelationsForTemplate back-fills every api field on the template from
// existing relation edges (the "Synchronize from relations" utility), returning
// the summed result. See Manager.SyncRelationsForTemplate. Idempotent.
func (s *Service) SyncRelationsForTemplate(template string) (ImportRelationResult, error) {
	return s.m.SyncRelationsForTemplate(template)
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

// SequenceOrder returns the collection's datafiles in sequence order, so the
// studio list can render a presentation template as an ordered deck.
func (s *Service) SequenceOrder(templateName string) ([]string, error) {
	return s.m.SequenceOrder(templateName)
}

// ReorderSequence moves one record to its new position (drag-to-reorder),
// writing only the moved record's sequence value unless the gaps force a
// renumber. See Manager.ReorderSequence.
func (s *Service) ReorderSequence(templateName, movedDatafile string, orderedDatafiles []string) (ReorderResult, error) {
	return s.m.ReorderSequence(templateName, movedDatafile, orderedDatafiles)
}

// NormalizeSequence re-spreads the collection to clean step spacing (the
// "Normalize" action). See Manager.NormalizeSequence.
func (s *Service) NormalizeSequence(templateName string) (ReorderResult, error) {
	return s.m.NormalizeSequence(templateName)
}

// EnsureFormDir creates the per-template storage folder. Vue calls
// this on first list against a freshly-created template.
func (s *Service) EnsureFormDir(templateName string) error {
	return s.m.EnsureFormDir(templateName)
}
