package template

import "testing"

// Virtual is an architectural concept: a field type that participates
// in template layout (key, label, description, validation, rendering)
// but does NOT seed a slot in storage.Form.Data. The virtual types are
// `facet` (reads/writes meta.facets[<key>]) and `formula` (writes a
// formula's output into another data field's slot, on load or on save).
//
// These tests pin the registry contract so storage.Sanitize (and any
// future virtual type) can rely on a single helper.

func TestIsVirtualFieldType_FacetAndFormulaAreVirtual(t *testing.T) {
	virtual := stringSet("facet", "formula")
	for id := range fieldDescriptors {
		got := IsVirtualFieldType(id)
		want := virtual[id]
		if got != want {
			t.Errorf("IsVirtualFieldType(%q) = %v, want %v", id, got, want)
		}
	}
	if IsVirtualFieldType("not-a-type") {
		t.Error("unknown type must not be reported as virtual")
	}
}

func TestAbilities_Virtual_OnlyOnFacetAndFormula(t *testing.T) {
	allowed := stringSet("facet", "formula")
	for id, def := range fieldDescriptors {
		got := def.Virtual
		want := allowed[id]
		if got != want {
			t.Errorf("type %q: Virtual = %v, want %v", id, got, want)
		}
	}
}

func TestRegistry_FacetHasExpectedAbilities(t *testing.T) {
	def, ok := fieldDescriptors["facet"]
	if !ok {
		t.Fatal("registry is missing the facet type")
	}
	if !def.Virtual {
		t.Error("facet must have Virtual: true")
	}
	want := Abilities{
		Key: true, Type: true, Label: true, Description: true,
		Default: true, Options: false, SummaryField: false, PrimaryKey: false,
		ExpressionItem: true, TwoColumn: true, Collapsible: false,
		Readonly: false, Format: true, UseInStatistics: false,
		FacetKey: true,
	}
	if def.Abilities != want {
		t.Errorf("facet abilities = %+v, want %+v", def.Abilities, want)
	}
}

func TestRegistry_FormulaHasExpectedAbilities(t *testing.T) {
	def, ok := fieldDescriptors["formula"]
	if !ok {
		t.Fatal("registry is missing the formula type")
	}
	if !def.Virtual {
		t.Error("formula must have Virtual: true")
	}
	want := Abilities{
		Key: true, Type: true, Label: true, Description: true,
		Default: false, Options: false, SummaryField: false, PrimaryKey: false,
		ExpressionItem: false, TwoColumn: true, Collapsible: false,
		Readonly: false, Format: false, UseInStatistics: false,
		FacetKey: false,
	}
	if def.Abilities != want {
		t.Errorf("formula abilities = %+v, want %+v", def.Abilities, want)
	}
}
