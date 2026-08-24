package query

import "errors"

// Service is the Wails-facing layer over Manager. The studio query panel
// calls Run; the REST endpoint calls Manager.Run directly (it holds the
// manager, not the service).
type Service struct {
	m     *Manager
	store *Store
}

func NewService(m *Manager, store *Store) *Service { return &Service{m: m, store: store} }

// Sources lists the queryable sources for a template (fields, table
// columns, facets) with their capabilities, for the query panel's column,
// filter, group and measure pickers. Backend-owned so the frontend never
// reimplements the source universe.
func (s *Service) Sources(template string) ([]SourceInfo, error) {
	return s.m.Sources(template)
}

// Run executes a query Spec and returns the typed Result. Errors
// (bad template, out-of-range column, invalid filter op) surface as the
// returned error so the frontend toast shows the backend message.
func (s *Service) Run(spec Spec) (Result, error) {
	return s.m.Run(spec)
}

// Explain returns a read-only SQL-shaped preview of a spec, rendered by
// the backend so it stays truthful to what the engine runs.
func (s *Service) Explain(spec Spec) (string, error) {
	return s.m.Explain(spec)
}

// FilterOps returns the comparison operators a filter may use, in display
// order. Backend-owned (sourced from the datacore) so the query panel's
// operator picker can't drift from what the engine accepts.
func (s *Service) FilterOps() []string {
	return FilterOps
}

// ListSavedQueries returns the template's saved queries, name-ascending, for
// the query panel's picker.
func (s *Service) ListSavedQueries(template string) ([]SavedQuery, error) {
	if s.store == nil {
		return nil, errors.New("query: saved queries unavailable")
	}
	return s.store.List(template)
}

// GetSavedQuery returns one saved query, spec included, so the panel can
// rebuild the builder from it. Location comes from the list row: the same id
// may exist locally and in the shared tree.
func (s *Service) GetSavedQuery(template, id, location string) (SavedQuery, error) {
	if s.store == nil {
		return SavedQuery{}, errors.New("query: saved queries unavailable")
	}
	return s.store.Get(template, id, location)
}

// SaveQuery stores a named spec and returns the stored record (id derived from
// the name when the caller left it empty, location from the profile setting
// when the record does not already have one).
func (s *Service) SaveQuery(q SavedQuery) (SavedQuery, error) {
	if s.store == nil {
		return SavedQuery{}, errors.New("query: saved queries unavailable")
	}
	return s.store.Save(q)
}

// DeleteSavedQuery removes a saved query from one location; deleting a missing
// one succeeds.
func (s *Service) DeleteSavedQuery(template, id, location string) error {
	if s.store == nil {
		return errors.New("query: saved queries unavailable")
	}
	return s.store.Delete(template, id, location)
}

// SavedQueryLocations returns the closed location set, in display order, so
// the settings picker renders the backend's list instead of restating it.
func (s *Service) SavedQueryLocations() []string { return StoreLocations }

// DefaultSavedQueryLocation is where a new save lands under the current
// profile setting. The query panel shows it so "Save" is never a surprise.
func (s *Service) DefaultSavedQueryLocation() string {
	if s.store == nil {
		return LocationLocal
	}
	return s.store.DefaultLocation()
}
