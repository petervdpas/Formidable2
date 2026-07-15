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

// boardTemplate is a valid plan board: Project Mode on, a project axis, and an
// event wrapped in a loop named "events". The helper keeps the rule tests honest.
func boardTemplate() *Template {
	return &Template{
		ProjectMode: true,
		Fields: []Field{
			{Key: "project", Type: "project"},
			{Key: "events", Type: "loopstart"},
			{Key: "event", Type: "event"},
			{Key: "events", Type: "loopstop"},
		},
	}
}

// event is the plan-board time-bar: forced key, wrapped in an "events" loop, and
// gated by Project Mode (a template flag), NOT by a sibling field or collection.
func TestEventFieldDescriptor_KeyReadonlyNoStructuralRequirements(t *testing.T) {
	got, ok := fieldDescriptors["event"]
	if !ok {
		t.Fatalf("event descriptor missing")
	}
	if !got.KeyReadonly {
		t.Errorf("event key must be read-only (forced singleton)")
	}
	if got.RequiresCollection {
		t.Errorf("event must not require collection mode")
	}
}

func TestValidate_EventNeedsEventsLoop(t *testing.T) {
	// A bare event (no enclosing loop) is flagged.
	if errs := Validate(&Template{ProjectMode: true, Fields: []Field{
		{Key: "project", Type: "project"},
		{Key: "event", Type: "event"},
	}}); !hasErr(errs, "event-needs-events-loop") {
		t.Errorf("event outside a loop should be flagged; got %+v", errs)
	}
	// An event in a differently-named loop is flagged.
	if errs := Validate(&Template{ProjectMode: true, Fields: []Field{
		{Key: "project", Type: "project"},
		{Key: "items", Type: "loopstart"},
		{Key: "event", Type: "event"},
		{Key: "items", Type: "loopstop"},
	}}); !hasErr(errs, "event-needs-events-loop") {
		t.Errorf("event in a non-\"events\" loop should be flagged; got %+v", errs)
	}
	// An event inside an "events" loop is fine.
	if errs := Validate(boardTemplate()); hasErr(errs, "event-needs-events-loop") {
		t.Errorf("event in an \"events\" loop should be fine; got %+v", errs)
	}
}

func TestValidate_EventNeedsProjectMode(t *testing.T) {
	// An event on a template not in Project Mode is flagged.
	if errs := Validate(&Template{Fields: []Field{
		{Key: "events", Type: "loopstart"},
		{Key: "event", Type: "event"},
		{Key: "events", Type: "loopstop"},
	}}); !hasErr(errs, "event-needs-project-mode") {
		t.Errorf("event without Project Mode should be flagged; got %+v", errs)
	}
	// A full board (Project Mode + project + events loop) has no such error.
	if errs := Validate(boardTemplate()); hasErr(errs, "event-needs-project-mode") {
		t.Errorf("event in Project Mode should be fine; got %+v", errs)
	}
}

func TestValidate_ProjectModeNeedsProject(t *testing.T) {
	// Project Mode on without a project field is flagged.
	if errs := Validate(&Template{ProjectMode: true, Fields: []Field{
		{Key: "id", Type: "guid"},
	}}); !hasErr(errs, "project-mode-needs-project") {
		t.Errorf("Project Mode without a project field should be flagged; got %+v", errs)
	}
	// With a project field, no such error.
	if errs := Validate(&Template{ProjectMode: true, Fields: []Field{
		{Key: "project", Type: "project"},
	}}); hasErr(errs, "project-mode-needs-project") {
		t.Errorf("Project Mode with a project field should be fine; got %+v", errs)
	}
	// Project Mode off needs nothing (asymmetric gate).
	if errs := Validate(&Template{Fields: []Field{{Key: "t", Type: "text"}}}); hasErr(errs, "project-mode-needs-project") {
		t.Errorf("Project Mode off should require nothing; got %+v", errs)
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
