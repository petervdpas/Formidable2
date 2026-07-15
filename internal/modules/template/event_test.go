package template

import "testing"

func TestParseEventDoc_RoundTrip(t *testing.T) {
	in := map[string]any{"start": "2026-06-27", "end": "2026-08-16", "kind": "task", "resource": "Ferry"}
	doc, err := ParseEventDoc(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Start != "2026-06-27" || doc.End != "2026-08-16" || doc.Kind != "task" || doc.Resource != "Ferry" {
		t.Errorf("round-trip mismatch: %+v", doc)
	}
}

func TestParseEventDoc_Nil(t *testing.T) {
	doc, err := ParseEventDoc(nil)
	if err != nil {
		t.Fatalf("parse nil: %v", err)
	}
	if doc != (EventDoc{}) {
		t.Errorf("nil should decode to empty doc, got %+v", doc)
	}
}

func TestParseEventDoc_WrongInnerType(t *testing.T) {
	// A numeric resource can't unmarshal into a string field: caller treats as drift.
	if _, err := ParseEventDoc(map[string]any{"resource": 7}); err == nil {
		t.Error("expected error decoding numeric resource, got nil")
	}
}

func TestIsEventKind(t *testing.T) {
	for _, k := range []string{"task", "milestone", "absence"} {
		if !IsEventKind(k) {
			t.Errorf("%q should be a valid kind", k)
		}
	}
	for _, k := range []string{"", "vacation", "TASK"} {
		if IsEventKind(k) {
			t.Errorf("%q should not be a valid kind", k)
		}
	}
}

func TestReservedKey_Event(t *testing.T) {
	// "event" is owned by the event type, like "slide"/"slideset"/"id".
	if errs := Validate(&Template{Fields: []Field{{Key: "event", Type: "text"}}}); !hasErr(errs, "reserved-key") {
		t.Errorf("text field keyed \"event\" should be flagged reserved-key; got %+v", errs)
	}
	if errs := Validate(&Template{Fields: []Field{{Key: "event", Type: "event"}}}); hasErr(errs, "reserved-key") {
		t.Errorf("event field keyed \"event\" should NOT be reserved-key; got %+v", errs)
	}
}

// event is the plan-board time-bar: like slideset it needs a collection and a
// companion field, but the roles are flipped: the item (event) requires the
// container (project), not the other way round.
func TestEventFieldDescriptor_RequiresCollectionAndProject(t *testing.T) {
	got, ok := fieldDescriptors["event"]
	if !ok {
		t.Fatalf("event descriptor missing")
	}
	if !got.KeyReadonly {
		t.Errorf("event key must be read-only (forced singleton)")
	}
	if !got.RequiresCollection {
		t.Errorf("events are a board's records, so event requires collection mode")
	}
	if !got.RequiresProject {
		t.Errorf("an event is a bar on a project axis, so it requires project mode")
	}
	if got.RequiresSlide {
		t.Errorf("event has nothing to do with slides")
	}
}

func TestValidate_EventNeedsCollection(t *testing.T) {
	// Without collection, an event field is flagged (even with a project present).
	if errs := Validate(&Template{Fields: []Field{
		{Key: "project", Type: "project"},
		{Key: "event", Type: "event"},
	}}); !hasErr(errs, "event-needs-collection") {
		t.Errorf("event without collection should be flagged; got %+v", errs)
	}
	// With collection and a project, no such error.
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{
		{Key: "id", Type: "guid"},
		{Key: "project", Type: "project"},
		{Key: "event", Type: "event"},
	}}); hasErr(errs, "event-needs-collection") {
		t.Errorf("event on a collection should be fine; got %+v", errs)
	}
}

func TestValidate_EventNeedsProject(t *testing.T) {
	// An event without a project field is flagged (needs the shared axis).
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{
		{Key: "id", Type: "guid"},
		{Key: "event", Type: "event"},
	}}); !hasErr(errs, "event-needs-project") {
		t.Errorf("event without a project field should be flagged; got %+v", errs)
	}
	// With a project field present, no such error.
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{
		{Key: "id", Type: "guid"},
		{Key: "project", Type: "project"},
		{Key: "event", Type: "event"},
	}}); hasErr(errs, "event-needs-project") {
		t.Errorf("event with a project field should be fine; got %+v", errs)
	}
	// A lone project (no event) is fine: the gate is asymmetric.
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{
		{Key: "id", Type: "guid"},
		{Key: "project", Type: "project"},
	}}); hasErr(errs, "event-needs-project") {
		t.Errorf("a lone project board should not require an event; got %+v", errs)
	}
}

func TestEventKinds_DefensiveCopy(t *testing.T) {
	a := EventKinds()
	if len(a) != 3 {
		t.Fatalf("want 3 kinds, got %d", len(a))
	}
	a[0].Name = "mutated"
	if b := EventKinds(); b[0].Name != EventKindTask {
		t.Errorf("EventKinds not a defensive copy: %q", b[0].Name)
	}
}
