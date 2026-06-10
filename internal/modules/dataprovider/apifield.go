package dataprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/nav"
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
	datafile, err := m.resolveAPIFieldDatafile(sourceTemplate, guid)
	if err != nil {
		return nil, err
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

// APIFieldTitle is the collapsed-card title for one referenced record: the live
// value of the FIRST mapped column (so the author controls it by ordering the
// editable fields), falling back to the target's own collection title and then
// the bare guid. columnKeys is the api field's map keys in author order.
func (m *Manager) APIFieldTitle(ctx context.Context, sourceTemplate, guid string, columnKeys []string) (string, error) {
	if len(columnKeys) > 0 {
		row, err := m.FetchAPIFieldRow(ctx, sourceTemplate, guid, columnKeys[:1])
		if err != nil {
			return "", err
		}
		if s := apiFieldTitleString(row[columnKeys[0]]); s != "" {
			return s, nil
		}
	}
	if it, ok, _ := m.ResolveCollectionByID(ctx, sourceTemplate, guid); ok && it != nil && it.Title != "" {
		return it.Title, nil
	}
	return guid, nil
}

// apiFieldTitleString renders a column value as a one-line title. Scalars
// stringify directly; a list of scalars joins with ", "; anything else (nested
// shapes) yields "" so the caller falls back to the collection title.
func apiFieldTitleString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case bool, int, int64, float64, float32:
		return fmt.Sprint(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			switch e.(type) {
			case map[string]any, []any:
				return ""
			}
			if s := apiFieldTitleString(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return ""
	}
}

// resolveAPIFieldDatafile validates the source collection and resolves a guid to
// its datafile via the index. Shared by the column reader and the link builder so
// both agree on the same collection precondition and guid->datafile mapping.
func (m *Manager) resolveAPIFieldDatafile(sourceTemplate, guid string) (string, error) {
	tpls, err := m.idx.ListTemplates()
	if err != nil {
		return "", fmt.Errorf("api-field: list templates: %w", err)
	}
	var found, enableCollection bool
	for _, t := range tpls {
		if t.Filename == sourceTemplate {
			found = true
			enableCollection = t.EnableCollection && t.GuidField != ""
			break
		}
	}
	if !found {
		return "", fmt.Errorf("%w: %q", ErrAPIFieldTemplateNotFound, sourceTemplate)
	}
	if !enableCollection {
		return "", fmt.Errorf("%w: %q", ErrAPIFieldCollectionDisabled, sourceTemplate)
	}

	rows, err := m.idx.ListForms(sourceTemplate, index.QueryOpts{})
	if err != nil {
		return "", fmt.Errorf("api-field: list forms: %w", err)
	}
	for _, r := range rows {
		if r.ID == guid {
			return r.Filename, nil
		}
	}
	return "", fmt.Errorf("%w: %q in %q", ErrAPIFieldGuidNotFound, guid, sourceTemplate)
}

// ResolveAPIFieldLink resolves (sourceTemplate, guid) to the canonical
// formidable://<template>:<datafile> deep link for the referenced record. One
// builder shared by the Handlebars card render and the form-side "Go to record".
func (m *Manager) ResolveAPIFieldLink(ctx context.Context, sourceTemplate, guid string) (string, error) {
	datafile, err := m.resolveAPIFieldDatafile(sourceTemplate, guid)
	if err != nil {
		return "", err
	}
	return nav.MakeHref(&nav.Target{Template: sourceTemplate, Datafile: datafile}), nil
}
