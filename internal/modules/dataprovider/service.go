package dataprovider

import (
	"context"
	"errors"
)

// Service is the Wails-bound facade. Today it surfaces only the
// api-field row reader (the picker UI in FieldEditModal calls
// FetchAPIFieldRow when the user picks a guid). The rest of
// dataprovider.Manager stays internal - it's read by the wiki HTTP
// server and the api Handler directly.
//
// The full Manager surface is intentionally NOT exposed: most of it
// is wiki-shaped (markdown+HTML pages, collection listings with
// hrefs) and would duplicate what the wiki server already serves.
// Frontend consumers that need a list of records for a picker can
// call the existing api Handler over loopback (via AssetMiddleware)
// or - when that's still too heavy - we add narrow per-need methods
// here, one at a time.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// APIFieldRowResult is the Wails-friendly response shape for
// FetchAPIFieldRow. Errors are surfaced as a stable Kind string so
// the frontend can branch without parsing free-form messages.
//
// Kinds:
//   - ""                     - success; Row is the projected row
//   - "template-not-found"   - sourceTemplate does not exist
//   - "collection-disabled"  - sourceTemplate exists but collection-mode is off
//   - "guid-not-found"       - guid is not present in that collection
//   - "internal"             - anything else (Message carries the detail)
type APIFieldRowResult struct {
	Row     map[string]any `json:"row,omitempty"`
	Kind    string         `json:"kind,omitempty"`
	Message string         `json:"message,omitempty"`
}

// ListCollectionTemplates returns the subset of templates that have
// collection-mode enabled (and therefore can be referenced by an
// api-typed field). The api-field editor populates its source-template
// dropdown from this list.
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

// FetchAPIFieldRow projects one source-collection record into a flat
// row keyed by the requested column keys. See manager method for
// flatten rules. Always returns a non-nil result; on error, Row is
// nil and Kind is non-empty.
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

// APIFieldRefetchResultDTO mirrors APIFieldRefetchResult but carries
// the same Kind/Message error fields as APIFieldRowResult so the
// frontend can branch uniformly. On success Row + Drift are populated;
// on error, both are zero and Kind is non-empty.
type APIFieldRefetchResultDTO struct {
	Row     map[string]any  `json:"row,omitempty"`
	Drift   []APIFieldDrift `json:"drift,omitempty"`
	Kind    string          `json:"kind,omitempty"`
	Message string          `json:"message,omitempty"`
}

// RefetchAPIFieldRow re-fetches the current projected row and returns
// it alongside the diff against `stored`. The host form should render
// Row as the new truth; Drift carries per-column entries for anything
// that changed since last save (including new columns added to the
// field's Map[]). A nil/empty stored is treated as "first refetch" -
// every non-nil current column counts as drift-from-zero.
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

// apiFieldErrorKind maps the structured api-field errors to the
// stable Kind strings the frontend branches on. Centralised so both
// service methods stay in lock-step if a new error category is added.
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
