package template

import "testing"

// The sequence field is a constrained integer ordering field. It mirrors the
// number descriptor but defaults its step to 10 (sparse numbering so a slide
// can be inserted between 10 and 20 without renumbering).
func TestSequenceFieldDescriptor_HasFixedStepRowDefaultingToTen(t *testing.T) {
	defs := AllFieldTypes()
	var got *FieldDescriptor
	for i := range defs {
		if defs[i].ID == "sequence" {
			d := defs[i]
			got = &d
			break
		}
	}
	if got == nil {
		t.Fatalf("sequence descriptor missing")
	}
	if !got.Abilities.Options {
		t.Fatalf("sequence must allow Options (step row)")
	}
	if got.OptionsShape == nil || len(got.OptionsShape.Rows) != 1 {
		t.Fatalf("sequence must advertise exactly one fixed option row (step); got %+v", got.OptionsShape)
	}
	row := got.OptionsShape.Rows[0]
	if row.Defaults["value"] != "step" {
		t.Errorf("step row locked value should be 'step'; got %v", row.Defaults["value"])
	}
	if row.Defaults["label"] != "10" {
		t.Errorf("sequence step default should be '10' (sparse numbering); got %v", row.Defaults["label"])
	}
	if len(got.OptionsShape.LockedColumns) != 1 || got.OptionsShape.LockedColumns[0] != "value" {
		t.Errorf("sequence should lock the value column; got %+v", got.OptionsShape.LockedColumns)
	}
	// Ordering is not identity: a sequence is never a primary key.
	if got.Abilities.PrimaryKey {
		t.Errorf("sequence must not be a primary key")
	}
	// The type dropdown gates on this: sequence only appears once collection is on.
	if !got.RequiresCollection {
		t.Errorf("sequence must declare RequiresCollection so the editor gates it")
	}
	if fieldDescriptors["number"].RequiresCollection {
		t.Errorf("number must not require collection")
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
