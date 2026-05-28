package storage

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// A virtual field (template.IsVirtualFieldType) does not occupy a slot
// in Form.Data. The first virtual type is "facet"; its value lives in
// Meta.Facets[<facet_key>] instead. Sanitize must skip virtual fields
// when seeding defaults so the data map stays free of orphan keys.

func TestSanitize_VirtualFacetFieldDoesNotSeedDataSlot(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio"},
	}
	raw := map[string]any{
		"title": "Hello",
		"_meta": map[string]any{
			"facets": map[string]any{
				"status": map[string]any{"set": true, "selected": "OPEN"},
			},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if _, ok := out.Data["status_inline"]; ok {
		t.Errorf("virtual facet field must not seed a data slot; got Data=%#v", out.Data)
	}
	if got, want := out.Data["title"], "Hello"; got != want {
		t.Errorf("title = %v, want %q", got, want)
	}
	if got := out.Meta.Facets["status"]; !got.Set || got.Selected != "OPEN" {
		t.Errorf("meta.facets[status] = %+v, want Set:true Selected:OPEN", got)
	}
}

func TestSanitize_VirtualFacetFieldWithDataKeyCollisionStillSkipsData(t *testing.T) {
	// Even if a caller sends a stray entry under the virtual field's key
	// in `data`, Sanitize must not adopt it. The virtual field has no
	// data slot, period.
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio"},
	}
	raw := map[string]any{
		"title":         "Hello",
		"status_inline": "should-be-ignored",
		"_meta":         map[string]any{},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if _, ok := out.Data["status_inline"]; ok {
		t.Errorf("stray data slot for virtual field must be dropped; got Data=%#v", out.Data)
	}
}

func TestSanitize_StrayDataKeyNotInTemplateIsAlreadyIgnored(t *testing.T) {
	// Sanity guard: the existing per-template-field loop already only
	// seeds keys it knows about. This pins that behavior in case a
	// future refactor flips it - the virtual-field guarantee above
	// depends on it.
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title":   "Hi",
		"unknown": "ignored",
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if _, ok := out.Data["unknown"]; ok {
		t.Errorf("only template-declared keys should land in Data; got %#v", out.Data)
	}
}
