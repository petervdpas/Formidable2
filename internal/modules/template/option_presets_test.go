package template

import "testing"

func TestService_TableColumnTypes_CanonicalSet(t *testing.T) {
	svc := &Service{}
	got := svc.TableColumnTypes()
	want := []string{"string", "number", "date", "bool", "dropdown", "reference"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %+v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("[%d] = %q, want %q (order matters)", i, got[i].Name, w)
		}
	}
}

func TestService_TableColumnTypes_NumberHasScalarStepSubRow(t *testing.T) {
	svc := &Service{}
	got := svc.TableColumnTypes()
	var number *TableColumnTypeDescriptor
	for i := range got {
		if got[i].Name == "number" {
			number = &got[i]
			break
		}
	}
	if number == nil {
		t.Fatal("number column type missing")
	}
	if number.SubRow == nil {
		t.Fatal("number column type has no sub-row (step option)")
	}
	if !number.SubRow.Scalar {
		t.Errorf("number sub-row must be scalar (single value, not value:label pairs)")
	}
	if number.SubRow.RowKey != "step" {
		t.Errorf("RowKey = %q, want %q", number.SubRow.RowKey, "step")
	}
	if number.SubRow.Default != "1" {
		t.Errorf("Default = %q, want %q", number.SubRow.Default, "1")
	}
	if len(number.SubRow.Entries) != 0 {
		t.Errorf("scalar sub-row must not declare pair Entries, got %d", len(number.SubRow.Entries))
	}
}

func TestService_ListItemTypes_CanonicalSet(t *testing.T) {
	svc := &Service{}
	got := svc.ListItemTypes()
	want := []string{"fixed", "custom"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %+v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestService_TableColumnTypes_ReturnsCopy(t *testing.T) {
	svc := &Service{}
	first := svc.TableColumnTypes()
	first[0].Name = "MUTATED"
	second := svc.TableColumnTypes()
	if second[0].Name == "MUTATED" {
		t.Errorf("caller mutation leaked into internal slice")
	}
}

func TestService_ListItemTypes_ReturnsCopy(t *testing.T) {
	svc := &Service{}
	first := svc.ListItemTypes()
	first[0].Name = "MUTATED"
	second := svc.ListItemTypes()
	if second[0].Name == "MUTATED" {
		t.Errorf("caller mutation leaked")
	}
}

func TestService_TableColumnTypes_StableOrder(t *testing.T) {
	svc := &Service{}
	a := svc.TableColumnTypes()
	b := svc.TableColumnTypes()
	for i := range a {
		if a[i].Name != b[i].Name {
			t.Errorf("[%d] order differs: %q vs %q", i, a[i].Name, b[i].Name)
		}
	}
}
