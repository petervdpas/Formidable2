package template

import "testing"

// Virtual is a new architectural concept: a field type that participates
// in template layout (key, label, description, validation, rendering)
// but does NOT seed a slot in storage.Form.Data. The first virtual type
// is `facet`, which reads/writes meta.facets[<key>] instead.
//
// These tests pin the registry contract so storage.Sanitize (and any
// future virtual type) can rely on a single helper.

func TestIsVirtualFieldType_OnlyFacetIsVirtual(t *testing.T) {
	virtual := stringSet("facet")
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

func TestAbilities_Virtual_OnlyOnFacet(t *testing.T) {
	allowed := stringSet("facet")
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
		ExpressionItem: false, TwoColumn: true, Collapsible: false,
		Readonly: false, Format: true, UseInStatistics: false,
		FacetKey: true,
	}
	if def.Abilities != want {
		t.Errorf("facet abilities = %+v, want %+v", def.Abilities, want)
	}
}
