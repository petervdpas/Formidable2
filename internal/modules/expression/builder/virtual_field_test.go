package builder

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// IsDisplayableFieldType is the single backend signal driving whether
// an expression_item field appears in the OutcomeEditor's display
// pickers (FieldValue / FieldLabel parts). Virtual types are excluded
// by design: their value is rendered by a dedicated widget (the facet
// chip) and concatenating it into a label string is redundant and
// visually noisy. Use them as predicate criteria instead.

func TestIsDisplayableFieldType_VirtualTypesExcluded(t *testing.T) {
	if IsDisplayableFieldType("facet") {
		t.Error("facet must NOT be displayable")
	}
}

func TestIsDisplayableFieldType_ScalarTypesIncluded(t *testing.T) {
	for _, ty := range []string{
		"text", "textarea", "number", "range", "date",
		"boolean", "dropdown", "radio",
	} {
		if !IsDisplayableFieldType(ty) {
			t.Errorf("%s must be displayable", ty)
		}
	}
}

func TestIsDisplayableFieldType_UnknownTypeIsNotDisplayable(t *testing.T) {
	// Unknown types never satisfy expression_item filtering on the
	// frontend either, so the conservative answer is false - keeps
	// future virtual types invisible until they're explicitly opted
	// in via the registry, regardless of any frontend slip.
	if IsDisplayableFieldType("mystery") {
		t.Error("unknown type must NOT be displayable")
	}
	if IsDisplayableFieldType("") {
		t.Error("empty type must NOT be displayable")
	}
}

// OptionsForField is the single backend source of the value/label pairs
// the predicate value-picker should offer for a given field. The
// frontend asks the backend rather than reading f.options itself, so
// virtual types (facet → resolve via facets[FacetKey]) and any future
// projection stay in one place.

func TestOptionsForField_FacetResolvesFromBoundFacet(t *testing.T) {
	field := template.Field{Key: "status_inline", Type: "facet", FacetKey: "status"}
	facets := []template.Facet{{
		Key:  "status",
		Icon: "fa-flag",
		Options: []template.FacetOption{
			{Label: "OPEN", Color: "blue"},
			{Label: "CLOSED", Color: "gray"},
		},
	}}
	got := OptionsForField(field, facets)
	want := []FieldOption{
		{Value: "OPEN", Label: "OPEN"},
		{Value: "CLOSED", Label: "CLOSED"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestOptionsForField_FacetUnknownBindingReturnsEmpty(t *testing.T) {
	field := template.Field{Key: "f", Type: "facet", FacetKey: "ghost"}
	facets := []template.Facet{{Key: "status", Icon: "fa-flag", Options: []template.FacetOption{{Label: "OPEN", Color: "blue"}}}}
	if got := OptionsForField(field, facets); len(got) != 0 {
		t.Errorf("unknown facet binding must return empty; got %+v", got)
	}
}

func TestOptionsForField_DropdownReadsFieldOptions(t *testing.T) {
	field := template.Field{
		Key:  "size",
		Type: "dropdown",
		Options: []any{
			map[string]any{"value": "S", "label": "Small"},
			map[string]any{"value": "L", "label": "Large"},
		},
	}
	got := OptionsForField(field, nil)
	want := []FieldOption{
		{Value: "S", Label: "Small"},
		{Value: "L", Label: "Large"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestOptionsForField_DropdownLabelFallsBackToValue(t *testing.T) {
	field := template.Field{
		Key:  "x",
		Type: "dropdown",
		Options: []any{
			map[string]any{"value": "S"}, // no label
			"raw-string",
		},
	}
	got := OptionsForField(field, nil)
	want := []FieldOption{
		{Value: "S", Label: "S"},
		{Value: "raw-string", Label: "raw-string"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
