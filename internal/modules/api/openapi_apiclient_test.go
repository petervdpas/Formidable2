package api

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// An api-client field is published as the snapshot it stores, so an external
// consumer reading /api/* gets the fetched values, not an opaque id it has no
// way to resolve.

func TestFieldToProperty_ApiClientPublishesTheSnapshot(t *testing.T) {
	key, schema := fieldToProperty(template.Field{
		Key: "customer", Type: "api-client",
		ClientID: "crm", Resource: "customers",
		Map: []template.APIMap{{Key: "city", Label: "City"}},
	})
	if key != "customer" {
		t.Fatalf("key = %q", key)
	}
	variants, ok := schema["oneOf"].([]any)
	if !ok || len(variants) != 2 {
		t.Fatalf("expected oneOf with a single and a to-many variant; got %#v", schema)
	}
	single, ok := variants[0].(map[string]any)
	if !ok || single["type"] != "object" {
		t.Fatalf("single variant is not an object: %#v", variants[0])
	}
	props, ok := single["properties"].(map[string]any)
	if !ok {
		t.Fatalf("no properties on the snapshot: %#v", single)
	}
	for _, want := range []string{"id", "label", "fields", "fetched"} {
		if _, has := props[want]; !has {
			t.Errorf("snapshot schema is missing %q: %#v", want, props)
		}
	}
	if schema["description"] == nil {
		t.Error("expected a type-specific description")
	}
}

// The projected keys are declared, so the schema names what a consumer can read.
func TestFieldToProperty_ApiClientDeclaresProjectedKeys(t *testing.T) {
	_, schema := fieldToProperty(template.Field{
		Key: "customer", Type: "api-client",
		ClientID: "crm", Resource: "customers",
		Map: []template.APIMap{{Key: "city"}, {Key: "country"}},
	})
	variants, _ := schema["oneOf"].([]any)
	single, _ := variants[0].(map[string]any)
	props, _ := single["properties"].(map[string]any)
	fields, ok := props["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields is not a schema: %#v", props["fields"])
	}
	inner, ok := fields["properties"].(map[string]any)
	if !ok {
		t.Fatalf("projected keys not declared: %#v", fields)
	}
	for _, want := range []string{"city", "country"} {
		if _, has := inner[want]; !has {
			t.Errorf("projected key %q missing: %#v", want, inner)
		}
	}
}
