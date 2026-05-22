package csv

import (
	"testing"
)

func TestSuggestMappings_ExactKeyMatch(t *testing.T) {
	fields := []FieldSpec{
		{Key: "name", Type: "text", Label: "Name"},
		{Key: "city", Type: "text", Label: "City"},
	}
	got := SuggestMappings([]string{"name", "city"}, fields)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Header != "name" || got[0].FieldKey != "name" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Header != "city" || got[1].FieldKey != "city" {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestSuggestMappings_CaseInsensitive(t *testing.T) {
	fields := []FieldSpec{{Key: "firstName", Type: "text", Label: "First Name"}}
	got := SuggestMappings([]string{"FIRSTNAME"}, fields)
	if got[0].FieldKey != "firstName" {
		t.Errorf("case-insensitive fail: %+v", got[0])
	}
}

func TestSuggestMappings_NormalizesWhitespaceUnderscoresDashes(t *testing.T) {
	fields := []FieldSpec{{Key: "unit_number", Type: "text", Label: "Unit Number"}}
	for _, header := range []string{"unit-number", "Unit Number", "unit number", "UnitNumber"} {
		got := SuggestMappings([]string{header}, fields)
		if got[0].FieldKey != "unit_number" {
			t.Errorf("header %q did not normalise to unit_number: %+v", header, got[0])
		}
	}
}

func TestSuggestMappings_MatchesLabelWhenKeyDoesnt(t *testing.T) {
	fields := []FieldSpec{{Key: "n", Type: "text", Label: "Customer Name"}}
	got := SuggestMappings([]string{"customer name"}, fields)
	if got[0].FieldKey != "n" {
		t.Errorf("label match failed: %+v", got[0])
	}
}

func TestSuggestMappings_NoMatchLeavesFieldKeyEmpty(t *testing.T) {
	fields := []FieldSpec{{Key: "a", Type: "text", Label: "Alpha"}}
	got := SuggestMappings([]string{"unrelated"}, fields)
	if got[0].FieldKey != "" {
		t.Errorf("expected empty key, got %+v", got[0])
	}
	// And the header is preserved verbatim.
	if got[0].Header != "unrelated" {
		t.Errorf("header not preserved: %+v", got[0])
	}
}

func TestSuggestMappings_ExcludedTypesNeverMatch(t *testing.T) {
	for _, ty := range ExcludedFieldTypes() {
		fields := []FieldSpec{{Key: "x", Type: ty, Label: "X"}}
		got := SuggestMappings([]string{"x"}, fields)
		if got[0].FieldKey != "" {
			t.Errorf("excluded type %q matched: %+v", ty, got[0])
		}
	}
}

func TestSuggestMappings_MultipleHeadersToSameField(t *testing.T) {
	// Two CSV columns named "name" both legitimately map to the "name"
	// field - the dialog's concat-with-separator UI handles the merge.
	fields := []FieldSpec{{Key: "name", Type: "text", Label: "Name"}}
	got := SuggestMappings([]string{"name", "name"}, fields)
	if got[0].FieldKey != "name" || got[1].FieldKey != "name" {
		t.Errorf("dup headers: %+v", got)
	}
}

func TestSuggestMappings_PreservesHeaderOrder(t *testing.T) {
	fields := []FieldSpec{
		{Key: "city", Type: "text", Label: "City"},
		{Key: "name", Type: "text", Label: "Name"},
	}
	got := SuggestMappings([]string{"name", "city"}, fields)
	if got[0].Header != "name" || got[1].Header != "city" {
		t.Errorf("order lost: %+v", got)
	}
}

func TestSuggestMappings_EmptyInputs(t *testing.T) {
	if got := SuggestMappings(nil, nil); len(got) != 0 {
		t.Errorf("nil/nil = %+v, want empty", got)
	}
	got := SuggestMappings([]string{"a"}, nil)
	if len(got) != 1 || got[0].FieldKey != "" {
		t.Errorf("no-fields: %+v", got)
	}
}

func TestMappableFields_FiltersExcluded(t *testing.T) {
	fields := []FieldSpec{
		{Key: "name", Type: "text"},
		{Key: "pic", Type: "image"},
		{Key: "items", Type: "loopstart"},
		{Key: "guid", Type: "guid"},
	}
	got := MappableFields(fields)
	if len(got) != 2 {
		t.Errorf("MappableFields = %+v, want 2", got)
	}
	for _, f := range got {
		if f.Type == "image" || f.Type == "loopstart" {
			t.Errorf("MappableFields kept excluded type: %+v", f)
		}
	}
}
