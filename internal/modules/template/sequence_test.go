package template

import "testing"

// The sequence field is a structural singleton like guid: the modal shows only
// Key + Field Type (no label/description/options), its key is forced and
// read-only, and it only appears once collection is on. Step is the sparse
// default (10) since there is no options row to change it.
func TestSequenceFieldDescriptor_IsMinimalGuidLikeSingleton(t *testing.T) {
	got, ok := fieldDescriptors["sequence"]
	if !ok {
		t.Fatalf("sequence descriptor missing")
	}
	if got.OptionsShape != nil {
		t.Errorf("sequence should advertise no options shape; got %+v", got.OptionsShape)
	}
	// Mirror guid's lean modal: every ability except Key/Type is off.
	a := got.Abilities
	if !a.Key || !a.Type {
		t.Errorf("sequence must keep Key + Type")
	}
	if a.Label || a.Description || a.Options || a.TwoColumn ||
		a.Default || a.PrimaryKey || a.ExpressionItem || a.UseInStatistics {
		t.Errorf("sequence modal must be minimal (guid-like); got %+v", a)
	}
	if !got.KeyReadonly {
		t.Errorf("sequence key must be read-only (forced singleton)")
	}
	if !got.RequiresCollection {
		t.Errorf("sequence must declare RequiresCollection so the editor gates it")
	}
	if fieldDescriptors["number"].RequiresCollection {
		t.Errorf("number must not require collection")
	}
	// Default step with no options row is the sparse 10.
	if SequenceStep(Field{Type: "sequence"}) != 10 {
		t.Errorf("sequence step should default to 10")
	}
}

func TestSequenceStep_DefaultsAndCustom(t *testing.T) {
	// No options -> sparse default of 10.
	if got := SequenceStep(Field{Type: "sequence"}); got != 10 {
		t.Errorf("default step = %d, want 10", got)
	}
	// Author-set step is honoured.
	custom := Field{Type: "sequence", Options: []any{
		map[string]any{"value": "step", "label": "5"},
	}}
	if got := SequenceStep(custom); got != 5 {
		t.Errorf("custom step = %d, want 5", got)
	}
	// Garbage / sub-1 step falls back to the default.
	bad := Field{Type: "sequence", Options: []any{
		map[string]any{"value": "step", "label": "abc"},
	}}
	if got := SequenceStep(bad); got != 10 {
		t.Errorf("bad step = %d, want default 10", got)
	}
}

func TestNormalize_ForcesSequenceKey(t *testing.T) {
	tpl := &Template{
		EnableCollection: true,
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "whatever", Type: "sequence"},
		},
	}
	Normalize(tpl)
	if got := tpl.Fields[1].Key; got != "sequence" {
		t.Errorf("sequence key = %q, want forced to \"sequence\" (guid-like singleton)", got)
	}
}

func TestValidate_SequenceWithoutCollection_Flagged(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "pos", Type: "sequence"}},
	})
	if !hasErr(errs, "sequence-needs-collection") {
		t.Errorf("expected sequence-needs-collection; got %+v", errs)
	}
}

func TestValidate_SequenceWithCollection_OK(t *testing.T) {
	errs := Validate(&Template{
		EnableCollection: true,
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "pos", Type: "sequence"},
		},
	})
	if hasErr(errs, "sequence-needs-collection") {
		t.Errorf("sequence on a collection is allowed; got %+v", errs)
	}
}

func TestValidate_MultipleSequenceFields_Flagged(t *testing.T) {
	errs := Validate(&Template{
		EnableCollection: true,
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "pos", Type: "sequence"},
			{Key: "pos2", Type: "sequence"},
		},
	})
	if !hasErr(errs, "multiple-sequence-fields") {
		t.Errorf("expected multiple-sequence-fields; got %+v", errs)
	}
}

func TestValidate_SingleSequenceField_OK(t *testing.T) {
	errs := Validate(&Template{
		EnableCollection: true,
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "pos", Type: "sequence"},
		},
	})
	if hasErr(errs, "multiple-sequence-fields") {
		t.Errorf("a single sequence field is allowed; got %+v", errs)
	}
}

func TestValidate_ReservedKeys(t *testing.T) {
	// A plain field claiming a reserved key is flagged.
	for _, key := range []string{"id", "sequence"} {
		errs := Validate(&Template{Fields: []Field{{Key: key, Type: "text"}}})
		if !hasErr(errs, "reserved-key") {
			t.Errorf("text field keyed %q should be flagged reserved-key; got %+v", key, errs)
		}
	}
	// The owning types are allowed their reserved key.
	ok := Validate(&Template{
		EnableCollection: true,
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "sequence", Type: "sequence"},
		},
	})
	if hasErr(ok, "reserved-key") {
		t.Errorf("guid/sequence may use their own reserved keys; got %+v", ok)
	}
}

func TestValidate_PresentationWithoutSequence_Flagged(t *testing.T) {
	errs := Validate(&Template{
		EnableCollection: true,
		Presentation:     true,
		Fields:           []Field{{Key: "id", Type: "guid"}},
	})
	if !hasErr(errs, "presentation-needs-sequence") {
		t.Errorf("expected presentation-needs-sequence; got %+v", errs)
	}
}

func TestValidate_PresentationWithSequence_OK(t *testing.T) {
	errs := Validate(&Template{
		EnableCollection: true,
		Presentation:     true,
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "pos", Type: "sequence"},
		},
	})
	if hasErr(errs, "presentation-needs-sequence") {
		t.Errorf("presentation with a sequence field is allowed; got %+v", errs)
	}
}
