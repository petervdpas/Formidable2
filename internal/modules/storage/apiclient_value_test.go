package storage

import "testing"

// The api-client field stores a snapshot of what it fetched, because the client
// definition and its vault secret live outside the synced tree. coerceAPIClientRef
// pins that shape so every consumer can read it without shape-sniffing.

func TestDefaultForType_ApiClientIsUnpicked(t *testing.T) {
	if got := defaultForType("api-client"); got != nil {
		t.Errorf("defaultForType(api-client) = %#v, want nil", got)
	}
}

func TestCoerceAPIClientRef_NilStaysNil(t *testing.T) {
	if got := coerceAPIClientRef(nil); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}

func TestCoerceAPIClientRef_EmptyStringIsUnpicked(t *testing.T) {
	if got := coerceAPIClientRef(""); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}

// A hand-authored yaml value may be a bare remote id; lift it to the snapshot
// shape so the renderer has something well-formed to read.
func TestCoerceAPIClientRef_BareIDLifted(t *testing.T) {
	got, ok := coerceAPIClientRef("cust-42").(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map", got)
	}
	if got["id"] != "cust-42" {
		t.Errorf("id = %#v, want cust-42", got["id"])
	}
	if _, isMap := got["fields"].(map[string]any); !isMap {
		t.Errorf("fields = %#v, want an empty map", got["fields"])
	}
}

func TestCoerceAPIClientRef_SnapshotPreserved(t *testing.T) {
	in := map[string]any{
		"id":      "cust-42",
		"label":   "Acme BV",
		"fields":  map[string]any{"city": "Utrecht"},
		"fetched": "2026-08-18T09:30:00Z",
	}
	got, ok := coerceAPIClientRef(in).(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map", got)
	}
	if got["label"] != "Acme BV" || got["fetched"] != "2026-08-18T09:30:00Z" {
		t.Errorf("snapshot metadata lost: %#v", got)
	}
	f, _ := got["fields"].(map[string]any)
	if f["city"] != "Utrecht" {
		t.Errorf("projected fields lost: %#v", got["fields"])
	}
}

// A snapshot without an id is not a pick, it is debris from a failed fetch.
func TestCoerceAPIClientRef_SnapshotWithoutIDIsUnpicked(t *testing.T) {
	in := map[string]any{"label": "orphan", "fields": map[string]any{}}
	if got := coerceAPIClientRef(in); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}

func TestCoerceAPIClientRef_MultiKeepsOrderAndDropsBlanks(t *testing.T) {
	in := []any{
		map[string]any{"id": "a", "label": "A"},
		"",
		map[string]any{"label": "no id"},
		"b",
	}
	got, ok := coerceAPIClientRef(in).([]any)
	if !ok {
		t.Fatalf("got %T, want slice", got)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	first, _ := got[0].(map[string]any)
	second, _ := got[1].(map[string]any)
	if first["id"] != "a" || second["id"] != "b" {
		t.Errorf("order or ids wrong: %#v", got)
	}
}

// Guards the whole point of the field: an api snapshot is an id-only reference,
// so it must never be mistaken for an api-client snapshot and vice versa.
func TestCoerceAPIRef_DoesNotApplyToApiClientSnapshots(t *testing.T) {
	snap := map[string]any{"id": "cust-42", "fields": map[string]any{"city": "Utrecht"}}
	if got := coerceAPIRef(snap); got != "cust-42" {
		t.Errorf("coerceAPIRef strips to the id: got %#v", got)
	}
	kept, _ := coerceAPIClientRef(snap).(map[string]any)
	if kept["fields"] == nil {
		t.Errorf("coerceAPIClientRef must keep the projected fields: %#v", kept)
	}
}
