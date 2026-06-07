package dataprovider

import (
	"context"
	"errors"
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// API-field structured errors; errors.Is against these, the Wails layer
// maps each to a stable code.
var (
	ErrAPIFieldTemplateNotFound   = errors.New("api-field: source template not found")
	ErrAPIFieldCollectionDisabled = errors.New("api-field: source template does not enable collection")
	ErrAPIFieldGuidNotFound       = errors.New("api-field: guid not found in source collection")
	ErrAPIFieldStorageMissing     = errors.New("api-field: storage adapter not wired")
)

// FetchAPIFieldRow resolves (sourceTemplate, guid) to a datafile via the
// index and returns each requested columnKey's source value verbatim
// (scalars and complex shapes alike, since .meta.json is already JSON).
// Every columnKey gets an entry; absent source keys map to nil, so callers
// can tell absent from explicit empty.
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
		// Stale index row: treat as guid-not-found, don't distinguish.
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
		row[key] = raw
	}
	return row, nil
}
