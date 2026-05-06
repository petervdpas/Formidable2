package template

import (
	"strings"
	"testing"
)

// YAML round-trip — locks the on-disk shape for properties whose
// nullable/optional semantics are fragile (`*bool`, `*int`). Adding
// a new such property should also add a case here.

func TestYAMLRoundTrip_CollapsibleTruePersists(t *testing.T) {
	src := &Template{
		Name: "x", Filename: "x.yaml",
		Fields: []Field{
			{Key: "li", Type: "list", Collapsible: boolPtr(true)},
		},
	}
	out, err := marshalYAML(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), "collapsible: true") {
		t.Errorf("expected `collapsible: true` in YAML, got:\n%s", out)
	}
	var got Template
	if err := unmarshalYAML(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(got.Fields))
	}
	if got.Fields[0].Collapsible == nil || *got.Fields[0].Collapsible != true {
		t.Errorf("Collapsible lost in round-trip: %+v", got.Fields[0].Collapsible)
	}
}

func TestYAMLRoundTrip_CollapsibleFalsePersists(t *testing.T) {
	// `false` must survive too — the `*bool` shape's whole point is
	// distinguishing "explicit false" from "not set". omitempty would
	// drop a bool false; the pointer keeps it.
	src := &Template{
		Name: "x", Filename: "x.yaml",
		Fields: []Field{
			{Key: "tb", Type: "table", Collapsible: boolPtr(false),
				Options: []any{map[string]any{"value": "c1", "label": "C1"}}},
		},
	}
	out, err := marshalYAML(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), "collapsible: false") {
		t.Errorf("expected `collapsible: false` in YAML, got:\n%s", out)
	}
	var got Template
	if err := unmarshalYAML(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Fields[0].Collapsible == nil {
		t.Fatalf("Collapsible should be non-nil false, got nil")
	}
	if *got.Fields[0].Collapsible != false {
		t.Errorf("Collapsible = %v, want false", *got.Fields[0].Collapsible)
	}
}

func TestYAMLRoundTrip_CollapsibleAbsentStaysAbsent(t *testing.T) {
	// No Collapsible set — must NOT appear in marshaled YAML and
	// must round-trip back as nil (omitempty + pointer behaviour).
	src := &Template{
		Name: "x", Filename: "x.yaml",
		Fields: []Field{
			{Key: "txt", Type: "text"},
		},
	}
	out, err := marshalYAML(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), "collapsible") {
		t.Errorf("absent collapsible should be omitted, got:\n%s", out)
	}
	var got Template
	if err := unmarshalYAML(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Fields[0].Collapsible != nil {
		t.Errorf("Collapsible should be nil after round-trip, got %v",
			*got.Fields[0].Collapsible)
	}
}
