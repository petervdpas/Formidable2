package dataprovider

import (
	"context"
	"errors"
)

// Service is the Wails-bound facade, surfacing only the picker-side
// api-field readers. The rest of Manager stays internal: it's wiki-shaped
// and the wiki HTTP server and api Handler read it directly. Narrow
// per-need methods get added here one at a time.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// APIFieldRowResult is the Wails response for FetchAPIFieldRow. Kind is ""
// on success or a stable error string (see apiFieldErrorKind) so the
// frontend branches without parsing Message.
type APIFieldRowResult struct {
	Row     map[string]any `json:"row,omitempty"`
	Kind    string         `json:"kind,omitempty"`
	Message string         `json:"message,omitempty"`
}

// ListCollectionItems returns a collection template's records (id + title +
// filename) for record pickers such as the relation linker. Empty when the
// template isn't a collection. Backend owns the list; the frontend renders it.
func (s *Service) ListCollectionItems(template string) []CollectionItem {
	page, err := s.m.ListCollection(context.Background(), template, CollectionListOpts{})
	if err != nil || page == nil {
		return []CollectionItem{}
	}
	return page.Items
}

// ListCollectionTemplates returns the collection-enabled templates an
// api-typed field can reference; the api-field editor's dropdown source.
func (s *Service) ListCollectionTemplates() []TemplateSummary {
	all, err := s.m.ListTemplates(context.Background())
	if err != nil {
		return []TemplateSummary{}
	}
	out := make([]TemplateSummary, 0, len(all))
	for _, t := range all {
		if t.EnableCollection && t.GuidField != "" {
			out = append(out, t)
		}
	}
	return out
}

// FetchAPIFieldRow projects one source-collection record into a flat row
// keyed by columnKeys. On error Row is nil and Kind is non-empty.
func (s *Service) FetchAPIFieldRow(sourceTemplate, guid string, columnKeys []string) APIFieldRowResult {
	row, err := s.m.FetchAPIFieldRow(context.Background(), sourceTemplate, guid, columnKeys)
	if err == nil {
		if row == nil {
			row = map[string]any{}
		}
		return APIFieldRowResult{Row: row}
	}
	return APIFieldRowResult{Kind: apiFieldErrorKind(err), Message: err.Error()}
}

// APIFieldRefetchResultDTO is APIFieldRefetchResult plus the Kind/Message
// error fields, for uniform frontend branching.
type APIFieldRefetchResultDTO struct {
	Row     map[string]any  `json:"row,omitempty"`
	Drift   []APIFieldDrift `json:"drift,omitempty"`
	Kind    string          `json:"kind,omitempty"`
	Message string          `json:"message,omitempty"`
}

// RefetchAPIFieldRow re-fetches the current row and returns it with the
// Drift against stored; a nil/empty stored counts every non-nil column as drift.
func (s *Service) RefetchAPIFieldRow(sourceTemplate, guid string, columnKeys []string, stored map[string]any) APIFieldRefetchResultDTO {
	res, err := s.m.RefetchAPIFieldRow(context.Background(), sourceTemplate, guid, columnKeys, stored)
	if err == nil {
		row := res.Row
		if row == nil {
			row = map[string]any{}
		}
		drift := res.Drift
		if drift == nil {
			drift = []APIFieldDrift{}
		}
		return APIFieldRefetchResultDTO{Row: row, Drift: drift}
	}
	return APIFieldRefetchResultDTO{Kind: apiFieldErrorKind(err), Message: err.Error()}
}

// apiFieldErrorKind maps the structured api-field errors to the stable
// Kind strings the frontend branches on.
func apiFieldErrorKind(err error) string {
	switch {
	case errors.Is(err, ErrAPIFieldTemplateNotFound):
		return "template-not-found"
	case errors.Is(err, ErrAPIFieldCollectionDisabled):
		return "collection-disabled"
	case errors.Is(err, ErrAPIFieldGuidNotFound):
		return "guid-not-found"
	default:
		return "internal"
	}
}
