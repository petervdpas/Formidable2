package dataprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// API-field structured errors. Callers should `errors.Is` against
// these rather than string-match. The Wails service layer maps each
// to a stable error code in its response shape.
var (
	ErrAPIFieldTemplateNotFound   = errors.New("api-field: source template not found")
	ErrAPIFieldCollectionDisabled = errors.New("api-field: source template does not enable collection")
	ErrAPIFieldGuidNotFound       = errors.New("api-field: guid not found in source collection")
	ErrAPIFieldStorageMissing     = errors.New("api-field: storage adapter not wired")
)

// FetchAPIFieldRow projects one row from a source collection-enabled
// template at picker time.
//
// Resolves (sourceTemplate, guid) → datafile via the index, loads the
// form's data via storage, and returns a flat map keyed by columnKey
// where each value is either a passthrough scalar or — for any non-
// scalar source value — a JSON-marshalled string. Scalars covered:
// string, bool, int/int64/float64, nil. Everything else flattens to
// JSON so the host form's storage stays scalar.
//
// The returned map always contains an entry for every requested
// columnKey; absent source keys produce nil values (so callers can
// tell "absent" apart from "explicit empty string"). An empty/nil
// columnKeys slice yields an empty (non-nil) row.
//
// Errors are structured (errors.Is-able) so the Wails layer can
// translate to stable codes:
//   - ErrAPIFieldTemplateNotFound when the source template is unknown
//   - ErrAPIFieldCollectionDisabled when the source isn't collection-mode
//   - ErrAPIFieldGuidNotFound when no form in that template carries the guid
//   - ErrAPIFieldStorageMissing when the manager wasn't wired with storage
func (m *Manager) FetchAPIFieldRow(ctx context.Context, sourceTemplate, guid string, columnKeys []string) (map[string]any, error) {
	tpls, err := m.idx.ListTemplates()
	if err != nil {
		return nil, fmt.Errorf("api-field: list templates: %w", err)
	}
	var found bool
	var enableCollection bool
	for _, t := range tpls {
		if t.Filename == sourceTemplate {
			found = true
			enableCollection = t.EnableCollection && t.GuidField != ""
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrAPIFieldTemplateNotFound, sourceTemplate)
	}
	if !enableCollection {
		return nil, fmt.Errorf("%w: %q", ErrAPIFieldCollectionDisabled, sourceTemplate)
	}

	// Resolve guid → datafile via the index.
	rows, err := m.idx.ListForms(sourceTemplate, index.QueryOpts{})
	if err != nil {
		return nil, fmt.Errorf("api-field: list forms: %w", err)
	}
	var datafile string
	for _, r := range rows {
		if r.ID == guid {
			datafile = r.Filename
			break
		}
	}
	if datafile == "" {
		return nil, fmt.Errorf("%w: %q in %q", ErrAPIFieldGuidNotFound, guid, sourceTemplate)
	}

	if m.sto == nil {
		return nil, ErrAPIFieldStorageMissing
	}
	form := m.sto.LoadForm(sourceTemplate, datafile)
	if form == nil {
		// Index has the row but storage can't find it — treat as
		// guid-not-found so callers don't have to distinguish a stale
		// index from a genuine miss. Logging is the storage layer's job.
		return nil, fmt.Errorf("%w: %q resolves to %q (gone from disk)",
			ErrAPIFieldGuidNotFound, guid, datafile)
	}

	row := make(map[string]any, len(columnKeys))
	for _, key := range columnKeys {
		raw, ok := form.Data[key]
		if !ok {
			row[key] = nil
			continue
		}
		flat, err := flattenAPIValue(raw)
		if err != nil {
			return nil, fmt.Errorf("api-field: flatten %q: %w", key, err)
		}
		row[key] = flat
	}
	return row, nil
}

// APIFieldDrift is one column where the host's stored value differs
// from the source's current value. Stored may be nil (the column was
// added to Map[] after the form was saved); Current may be nil (the
// source-side field was deleted/cleared).
type APIFieldDrift struct {
	Key     string `json:"key"`
	Stored  any    `json:"stored"`
	Current any    `json:"current"`
}

// APIFieldRefetchResult bundles a fresh projected row with the diff
// against a previously-stored row. Frontend renders Row as the new
// truth and uses Drift to flag what changed since last save.
type APIFieldRefetchResult struct {
	Row   map[string]any  `json:"row"`
	Drift []APIFieldDrift `json:"drift"`
}

// RefetchAPIFieldRow fetches the current projected row for
// (sourceTemplate, guid, columnKeys) and compares each column against
// stored. Returns the fresh row plus a Drift entry for every column
// whose value differs (including columns absent from stored or absent
// from current). A nil stored map is treated as empty (every non-nil
// current column counts as drift from zero).
//
// Comparison uses host-storage equality semantics: scalars compare by
// value, non-scalars compare by their JSON-flattened string (already
// produced by flattenAPIValue). Same-shaped values therefore round-
// trip cleanly even when the source's underlying type is complex.
func (m *Manager) RefetchAPIFieldRow(ctx context.Context, sourceTemplate, guid string, columnKeys []string, stored map[string]any) (*APIFieldRefetchResult, error) {
	row, err := m.FetchAPIFieldRow(ctx, sourceTemplate, guid, columnKeys)
	if err != nil {
		return nil, err
	}
	drift := []APIFieldDrift{}
	for _, key := range columnKeys {
		var s any
		if stored != nil {
			s = stored[key]
		}
		c := row[key]
		if !apiFieldValuesEqual(s, c) {
			drift = append(drift, APIFieldDrift{Key: key, Stored: s, Current: c})
		}
	}
	return &APIFieldRefetchResult{Row: row, Drift: drift}, nil
}

// apiFieldValuesEqual is the per-column equality rule. nil == nil;
// scalars compare by `==`; anything else (slice/map) is compared by
// its JSON-flattened string. Both inputs are expected to come from
// either flattenAPIValue or the caller's stored row (which itself was
// stamped through flattenAPIValue), so non-scalars are normally
// already strings — the json.Marshal fallback only covers the unusual
// case where a stored row carries a raw map/slice from somewhere else.
func apiFieldValuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if isAPIScalar(a) && isAPIScalar(b) {
		return a == b
	}
	as, aerr := flattenAPIValue(a)
	bs, berr := flattenAPIValue(b)
	if aerr != nil || berr != nil {
		return false
	}
	return as == bs
}

func isAPIScalar(v any) bool {
	switch v.(type) {
	case string, bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	}
	return false
}

// flattenAPIValue is the host-storage scalarisation rule: scalars
// (string/number/bool/nil) pass through; anything else is rendered as
// a JSON string. Keeps host-form storage flat regardless of how
// complex the source field's type was.
func flattenAPIValue(v any) (any, error) {
	switch v.(type) {
	case nil, string, bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return v, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
