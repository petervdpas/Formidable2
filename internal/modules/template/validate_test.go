package template

import (
	"testing"
)

// A template with two fields sharing a key loads fine but Validate flags exactly
// one duplicate-keys error naming that key.
func TestValidate_DuplicateFieldKeysFlagged(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "dup.yaml",
		"name: X\nfields:\n  - key: a\n    type: text\n  - key: a\n    type: text\n")
	tmpl, err := m.LoadTemplate("dup.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tmpl.Fields) != 2 {
		t.Fatalf("loaded fields = %d, want 2", len(tmpl.Fields))
	}
	errs := Validate(tmpl)
	// Two text fields sharing one key produce exactly one validation error.
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %+v", len(errs), errs)
	}
	dup := errs[0]
	if dup.Type != "duplicate-keys" {
		t.Fatalf("error type = %q, want duplicate-keys", dup.Type)
	}
	if len(dup.Keys) != 1 || dup.Keys[0] != "a" {
		t.Errorf("duplicate keys = %v, want [a]", dup.Keys)
	}
}

// A field missing its required type attribute surfaces missing-field-type
// carrying the offending key, not unknown-field-type.
func TestValidate_FieldMissingTypeFlagged(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "notype.yaml", "name: X\nfields:\n  - key: a\n")
	tmpl, err := m.LoadTemplate("notype.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	errs := Validate(tmpl)
	// A single untyped field produces exactly one error: missing-field-type.
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %+v", len(errs), errs)
	}
	miss := errs[0]
	if miss.Type != "missing-field-type" {
		t.Fatalf("error type = %q, want missing-field-type", miss.Type)
	}
	if miss.Key != "a" {
		t.Errorf("missing-field-type key = %q, want a", miss.Key)
	}
	// Empty type must not also report unknown-field-type (the code continues past it).
	for _, e := range errs {
		if e.Type == "unknown-field-type" {
			t.Errorf("unexpected unknown-field-type alongside missing-field-type: %+v", errs)
		}
	}
}

// An api field with no collection name surfaces exactly the
// api-collection-required error.
func TestValidate_ApiFieldMissingCollectionFlagged(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "api.yaml", "name: X\nfields:\n  - key: ref\n    type: api\n")
	tmpl, err := m.LoadTemplate("api.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	errs := Validate(tmpl)
	// A keyed api field with no collection produces exactly one error naming that key.
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %+v", len(errs), errs)
	}
	if errs[0].Type != "api-collection-required" {
		t.Fatalf("error type = %q, want api-collection-required", errs[0].Type)
	}
	if errs[0].Key != "ref" {
		t.Errorf("api-collection-required key = %q, want ref", errs[0].Key)
	}
}

// A document with no fields key at all parses without error but Validate rejects
// it as invalid-template since Fields is nil.
func TestValidate_NoFieldsKeyIsInvalidTemplate(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "nofields.yaml", "name: X\n")
	tmpl, err := m.LoadTemplate("nofields.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if tmpl.Fields != nil {
		t.Fatalf("expected nil Fields, got %+v", tmpl.Fields)
	}
	errs := Validate(tmpl)
	if len(errs) != 1 || errs[0].Type != "invalid-template" {
		t.Errorf("expected single invalid-template, got %+v", errs)
	}
}

// An explicitly empty fields list is the lower boundary: loads, zero fields,
// zero validation errors.
func TestValidate_EmptyFieldsListIsValid(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "empty.yaml", "name: X\nfields: []\n")
	tmpl, err := m.LoadTemplate("empty.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tmpl.Fields) != 0 {
		t.Fatalf("fields = %d, want 0", len(tmpl.Fields))
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Errorf("empty fields list should validate clean, got %+v", errs)
	}
}

// A template mixing one valid and one unknown-type field loads both fields and
// Validate flags exactly one unknown-field-type carrying the bad type and key.
func TestValidate_MixedKnownAndUnknownTypeField(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "mixed.yaml",
		"name: X\nfields:\n  - key: good\n    type: text\n  - key: bad\n    type: wizbang\n")
	tmpl, err := m.LoadTemplate("mixed.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tmpl.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(tmpl.Fields))
	}
	errs := Validate(tmpl)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %+v", len(errs), errs)
	}
	if errs[0].Type != "unknown-field-type" {
		t.Fatalf("error type = %q, want unknown-field-type", errs[0].Type)
	}
	if errs[0].Key != "bad" {
		t.Errorf("unknown-field-type key = %q, want bad", errs[0].Key)
	}
	if got := errs[0].Detail["type"]; got != "wizbang" {
		t.Errorf("detail type = %v, want wizbang", got)
	}
}

// Boundary: loop nesting exactly at maxLoopDepth (2) validates clean.
func TestValidate_LoopNestingAtMaxDepthIsClean(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "nest2.yaml",
		"name: X\nfields:\n"+
			"  - key: outer\n    type: loopstart\n"+
			"  - key: inner\n    type: loopstart\n"+
			"  - key: leaf\n    type: text\n"+
			"  - key: inner\n    type: loopstop\n"+
			"  - key: outer\n    type: loopstop\n")
	tmpl, err := m.LoadTemplate("nest2.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tmpl.Fields) != 5 {
		t.Fatalf("fields = %d, want 5", len(tmpl.Fields))
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Errorf("2-deep nest should validate clean, got %+v", errs)
	}
}

// Boundary: one level past maxLoopDepth surfaces exactly one excessive-loop-nesting
// error at the innermost key with the expected depth detail.
func TestValidate_LoopNestingOverMaxDepthFlagged(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "nest3.yaml",
		"name: X\nfields:\n"+
			"  - key: a\n    type: loopstart\n"+
			"  - key: b\n    type: loopstart\n"+
			"  - key: c\n    type: loopstart\n"+
			"  - key: c\n    type: loopstop\n"+
			"  - key: b\n    type: loopstop\n"+
			"  - key: a\n    type: loopstop\n")
	tmpl, err := m.LoadTemplate("nest3.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	errs := Validate(tmpl)
	count := 0
	var excessive ValidationError
	for _, e := range errs {
		if e.Type == "excessive-loop-nesting" {
			count++
			excessive = e
		}
	}
	if count != 1 {
		t.Fatalf("excessive-loop-nesting count = %d, want 1 (errs %+v)", count, errs)
	}
	if excessive.Key != "c" {
		t.Errorf("excessive-loop-nesting key = %q, want c", excessive.Key)
	}
	if got := excessive.Detail["depth"]; got != 3 {
		t.Errorf("excessive-loop-nesting depth = %v, want 3", got)
	}
}
