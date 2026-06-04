package template

import "testing"

func TestBooleanFieldDescriptor_HasFixedOptionsShape(t *testing.T) {
	defs := AllFieldTypes()
	var got *FieldDescriptor
	for i := range defs {
		if defs[i].ID == "boolean" {
			d := defs[i]
			got = &d
			break
		}
	}
	if got == nil {
		t.Fatalf("boolean descriptor missing")
	}
	if got.OptionsShape == nil {
		t.Fatalf("boolean should advertise a FixedOptionsShape; got nil")
	}
	if len(got.OptionsShape.Rows) != 2 {
		t.Fatalf("boolean shape must have exactly 2 rows (True/False); got %d", len(got.OptionsShape.Rows))
	}
	if got.OptionsShape.Rows[0].LabelKey != "common.true" || got.OptionsShape.Rows[1].LabelKey != "common.false" {
		t.Fatalf("row label keys wrong: %+v", got.OptionsShape.Rows)
	}
	if got.OptionsShape.Rows[0].Defaults["value"] != "true" {
		t.Fatalf("row 0 default value should be 'true'")
	}
	if got.OptionsShape.Rows[1].Defaults["value"] != "false" {
		t.Fatalf("row 1 default value should be 'false'")
	}
	if len(got.OptionsShape.LockedColumns) != 1 || got.OptionsShape.LockedColumns[0] != "value" {
		t.Fatalf("boolean should lock the value column; got %+v", got.OptionsShape.LockedColumns)
	}
}

func TestNumberFieldDescriptor_HasFixedStepRowDefaultingToOne(t *testing.T) {
	defs := AllFieldTypes()
	var got *FieldDescriptor
	for i := range defs {
		if defs[i].ID == "number" {
			d := defs[i]
			got = &d
			break
		}
	}
	if got == nil {
		t.Fatalf("number descriptor missing")
	}
	if !got.Abilities.Options {
		t.Fatalf("number must allow Options (step row)")
	}
	if got.OptionsShape == nil || len(got.OptionsShape.Rows) != 1 {
		t.Fatalf("number must advertise exactly one fixed option row (step); got %+v", got.OptionsShape)
	}
	row := got.OptionsShape.Rows[0]
	if row.LabelKey != "workspace.templates.field_edit.number.step" {
		t.Errorf("step row label key wrong: %q", row.LabelKey)
	}
	if row.Defaults["value"] != "step" {
		t.Errorf("step row locked value should be 'step'; got %v", row.Defaults["value"])
	}
	if row.Defaults["label"] != "1" {
		t.Errorf("step default should be '1' (integer-by-default); got %v", row.Defaults["label"])
	}
	if len(got.OptionsShape.LockedColumns) != 1 || got.OptionsShape.LockedColumns[0] != "value" {
		t.Errorf("number should lock the value column; got %+v", got.OptionsShape.LockedColumns)
	}
}

func TestRangeFieldDescriptor_HasFixedMinMaxStep(t *testing.T) {
	defs := AllFieldTypes()
	var got *FieldDescriptor
	for i := range defs {
		if defs[i].ID == "range" {
			d := defs[i]
			got = &d
			break
		}
	}
	if got == nil {
		t.Fatalf("range descriptor missing")
	}
	if got.OptionsShape == nil {
		t.Fatalf("range should advertise a FixedOptionsShape; got nil")
	}
	if len(got.OptionsShape.Rows) != 3 {
		t.Fatalf("range shape must have exactly 3 rows (min/max/step); got %d", len(got.OptionsShape.Rows))
	}
	want := []struct {
		key, val string
	}{{"min", "0"}, {"max", "10"}, {"step", "1"}}
	for i, w := range want {
		r := got.OptionsShape.Rows[i]
		if r.Defaults["value"] != w.key || r.Defaults["label"] != w.val {
			t.Fatalf("row %d should be value=%q label=%q; got %+v", i, w.key, w.val, r.Defaults)
		}
	}
	if len(got.OptionsShape.LockedColumns) != 1 || got.OptionsShape.LockedColumns[0] != "value" {
		t.Fatalf("range should lock the value column (only the number is editable); got %+v", got.OptionsShape.LockedColumns)
	}
}

func TestTableColumnTypes_DropdownAndBoolHaveSubRow(t *testing.T) {
	types := builtinTableColumnTypes
	byName := map[string]TableColumnTypeDescriptor{}
	for _, d := range types {
		byName[d.Name] = d
	}
	dd, ok := byName["dropdown"]
	if !ok {
		t.Fatalf("dropdown column type missing")
	}
	if dd.SubRow == nil || dd.SubRow.RowKey != "choices" {
		t.Fatalf("dropdown should expose a choices sub-row; got %+v", dd.SubRow)
	}
	if dd.SubRow.Entries != nil {
		t.Fatalf("dropdown sub-row is free-form; Entries should be nil")
	}
	bb, ok := byName["bool"]
	if !ok {
		t.Fatalf("bool column type missing")
	}
	if bb.SubRow == nil {
		t.Fatalf("bool should expose a sub-row")
	}
	if len(bb.SubRow.Entries) != 2 {
		t.Fatalf("bool sub-row must have exactly 2 entries; got %d", len(bb.SubRow.Entries))
	}
	if bb.SubRow.Entries[0].Value != "true" || bb.SubRow.Entries[1].Value != "false" {
		t.Fatalf("bool entries must lock value=true/value=false; got %+v", bb.SubRow.Entries)
	}
	if bb.SubRow.MaxEntries != 2 {
		t.Fatalf("bool sub-row MaxEntries should be 2")
	}
}

func TestTableColumnTypes_StringHasNoSubRow(t *testing.T) {
	// Free-form types must not carry sub-row metadata - keeps the
	// JSON minimal and the UI prediction simple.
	for _, d := range builtinTableColumnTypes {
		if d.Name == "string" || d.Name == "number" || d.Name == "date" || d.Name == "reference" {
			if d.SubRow != nil {
				t.Fatalf("%s should not carry a SubRow; got %+v", d.Name, d.SubRow)
			}
		}
	}
}
