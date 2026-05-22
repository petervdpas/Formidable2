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
