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

// APIFieldTitleResult is the Wails response for APIFieldTitle. Kind is "" on
// success (Title set) or a stable error string (see apiFieldErrorKind).
type APIFieldTitleResult struct {
	Title   string `json:"title,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
}

// APIFieldTitle returns the collapsed-card title for a referenced record: the
// first mapped column's value, with the collection title and guid as fallbacks.
// columnKeys are the api field's map keys in author order.
func (s *Service) APIFieldTitle(sourceTemplate, guid string, columnKeys []string) APIFieldTitleResult {
	title, err := s.m.APIFieldTitle(context.Background(), sourceTemplate, guid, columnKeys)
	if err == nil {
		return APIFieldTitleResult{Title: title}
	}
	return APIFieldTitleResult{Kind: apiFieldErrorKind(err), Message: err.Error()}
}

// APIFieldLinkResult is the Wails response for ResolveAPIFieldLink. Kind is ""
// on success (Href set) or a stable error string (see apiFieldErrorKind).
type APIFieldLinkResult struct {
	Href    string `json:"href,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
}

// ResolveAPIFieldLink returns the formidable://<template>:<datafile> deep link
// for a referenced record, so the form-side "Go to record" uses the same backend
// builder as the rendered card.
func (s *Service) ResolveAPIFieldLink(sourceTemplate, guid string) APIFieldLinkResult {
	href, err := s.m.ResolveAPIFieldLink(context.Background(), sourceTemplate, guid)
	if err == nil {
		return APIFieldLinkResult{Href: href}
	}
	return APIFieldLinkResult{Kind: apiFieldErrorKind(err), Message: err.Error()}
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
