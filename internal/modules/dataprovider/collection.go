package dataprovider

import (
	"context"
	"net/url"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// IsCollectionEnabled returns true when the named template opts in
// to collection-mode AND has a guid field (the index already pre-
// derived the latter - `EnableCollection` is only true here when
// both conditions hold). False for unknown templates.
func (m *Manager) IsCollectionEnabled(ctx context.Context, template string) bool {
	t, ok, err := m.GetTemplate(ctx, template)
	if err != nil || !ok {
		return false
	}
	return t.EnableCollection && t.GuidField != ""
}

// ListCollection returns a paginated, optionally q/tags-filtered view
// of forms for a collection-enabled template. Mirrors the original
// `listCollection` shape so the wiki HTTP routes can return JSON
// directly. Disabled templates surface a CollectionPage{Enabled:false}
// rather than an error - easier for HTTP callers to hand back as a 403.
func (m *Manager) ListCollection(ctx context.Context, template string, opts CollectionListOpts) (*CollectionPage, error) {
	if !m.IsCollectionEnabled(ctx, template) {
		return &CollectionPage{Enabled: false}, nil
	}

	rows, err := m.idx.ListForms(template, index.QueryOpts{
		Tags:    opts.Tags,
		// Limit/Offset are applied AFTER q substring filtering, so we
		// pull the full row set and paginate ourselves.
	})
	if err != nil {
		return nil, err
	}

	// q is case-insensitive substring across title + tags, matching
	// the wiki's old behaviour. Empty q skips the filter.
	if opts.Q != "" {
		needle := strings.ToLower(opts.Q)
		filtered := rows[:0:0]
		for _, r := range rows {
			haystack := strings.ToLower(r.Title)
			for _, t := range r.Tags {
				haystack += " " + strings.ToLower(t)
			}
			if strings.Contains(haystack, needle) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	// Drop rows without a GUID - they can't be addressed in /api/collections.
	addressable := rows[:0:0]
	for _, r := range rows {
		if r.ID != "" {
			addressable = append(addressable, r)
		}
	}
	rows = addressable

	// Facet filter: every requested key must match the row's facets
	// with set==true and selected==value (AND semantics). Reads come
	// straight off the FormRow now that the index materializes facets
	// in queryForms - no per-record disk traffic.
	if len(opts.Facets) > 0 {
		filtered := rows[:0:0]
		for _, r := range rows {
			rowFacets := facetMap(r.Facets)
			match := true
			for key, want := range opts.Facets {
				state, ok := rowFacets[key]
				if !ok || !state.Set || state.Selected != want {
					match = false
					break
				}
			}
			if match {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	total := len(rows)
	if opts.Offset > 0 {
		if opts.Offset >= len(rows) {
			rows = nil
		} else {
			rows = rows[opts.Offset:]
		}
	}
	if opts.Limit > 0 && len(rows) > opts.Limit {
		rows = rows[:opts.Limit]
	}

	stem := strings.TrimSuffix(template, ".yaml")
	items := make([]CollectionItem, len(rows))
	for i, r := range rows {
		items[i] = collectionItem(stem, r)
	}

	return &CollectionPage{
		Enabled:  true,
		Template: stem,
		Total:    total,
		Limit:    opts.Limit,
		Offset:   opts.Offset,
		Items:    items,
	}, nil
}

// ResolveCollectionByID finds one collection item by its GUID. The
// FormSummary-level ResolveByID returns a summary; this returns a
// CollectionItem with the wiki's standard hrefs already filled.
// Templates that don't have collection enabled always miss.
func (m *Manager) ResolveCollectionByID(ctx context.Context, template, id string) (*CollectionItem, bool, error) {
	if !m.IsCollectionEnabled(ctx, template) {
		return nil, false, nil
	}
	rows, err := m.idx.ListForms(template, index.QueryOpts{})
	if err != nil {
		return nil, false, err
	}
	stem := strings.TrimSuffix(template, ".yaml")
	for _, r := range rows {
		if r.ID == id {
			it := collectionItem(stem, r)
			return &it, true, nil
		}
	}
	return nil, false, nil
}

// CollectionRev returns the per-collection revision marker the wiki
// HTTP layer uses for ETag/Last-Modified. v1 piggy-backs on the
// index's global rev, which is correct (any write - even unrelated -
// invalidates the ETag, which is an acceptable conservative choice).
// We can refine to a per-template rev later by adding a tracked
// column to the templates table.
func (m *Manager) CollectionRev(_ context.Context, _ string) (int64, error) {
	return m.idx.Rev()
}

// facetMap turns the FormRow's flat facet slice into a key-indexed
// lookup so the filter loop reads cleanly. Returns nil for empty input
// so the caller can treat "no facets" as "no match" without a special
// case.
func facetMap(in []index.FormFacet) map[string]index.FormFacet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]index.FormFacet, len(in))
	for _, f := range in {
		out[f.Key] = f
	}
	return out
}

// collectionItem builds the public projection. Hrefs are constructed
// here once so HTTP handlers don't have to repeat the URL shape and
// risk drifting from the wiki's link contract.
func collectionItem(stem string, r index.FormRow) CollectionItem {
	return CollectionItem{
		Template: stem,
		ID:       r.ID,
		Filename: r.Filename,
		Title:    r.Title,
		Tags:     append([]string(nil), r.Tags...),
		HrefSelf: "/api/collections/" + url.PathEscape(stem) + "/" + url.PathEscape(r.ID),
		HrefHTML: "/template/" + url.PathEscape(stem) + "/form/" + url.PathEscape(r.Filename),
	}
}
