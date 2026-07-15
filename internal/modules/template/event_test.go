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
