package connection

import "time"

// The api-client field stores what it fetched instead of a bare remote id. The
// client definition, its spec file and its vault secret all live at app level,
// outside the tree that gigot/git syncs, so a peer pulling the repo has no way
// to resolve an id. The stored snapshot is the display truth; a live call is a
// refresh, never a dependency.
//
// The key names below are the on-disk contract, shared with
// storage.coerceAPIClientRef, which normalises the same four keys on load.

// Snapshot builds one stored pick from a fetched item. keys names the projected
// fields to keep (the field's Map); empty keeps identity only. An item with no
// id is not a pick and yields nil.
func Snapshot(item Item, keys []string, at time.Time) map[string]any {
	if item.ID == "" {
		return nil
	}
	fields := map[string]any{}
	for _, k := range keys {
		// Absent stays absent: a consumer can tell "the remote stopped sending
		// this" from "the value is empty".
		if v, ok := item.Fields[k]; ok {
			fields[k] = v
		}
	}
	return map[string]any{
		"id":      item.ID,
		"label":   item.Label,
		"fields":  fields,
		"fetched": at.UTC().Format(time.RFC3339),
	}
}

// Refresh replaces a stored snapshot with a freshly fetched one. A nil fresh
// item means the call failed or the record is gone: the stored copy stands, so
// an offline remote or a retired record never blanks a form.
func Refresh(stored map[string]any, fresh *Item, keys []string, at time.Time) map[string]any {
	if fresh == nil {
		return stored
	}
	next := Snapshot(*fresh, keys, at)
	if next == nil {
		return stored
	}
	return next
}
