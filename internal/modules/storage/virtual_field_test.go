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

func TestSanitize_FacetFieldDefaultSeedsMetaFacetsOnFreshRecord(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN"},
	}
	raw := map[string]any{"title": "Hello"}
	out := Sanitize(raw, fields, SanitizeOptions{})
	got, ok := out.Meta.Facets["status"]
	if !ok {
		t.Fatalf("expected meta.facets[status] to be seeded from Default; got Facets=%+v", out.Meta.Facets)
	}
	if !got.Set || got.Selected != "OPEN" {
		t.Errorf("seeded state = %+v, want Set:true Selected:OPEN", got)
	}
}

func TestSanitize_FacetFieldDefaultDoesNotOverrideExistingState(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN"},
	}
	raw := map[string]any{
		"title": "Hello",
		"_meta": map[string]any{
			"facets": map[string]any{
				"status": map[string]any{"set": true, "selected": "CLOSED"},
			},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if got := out.Meta.Facets["status"].Selected; got != "CLOSED" {
		t.Errorf("existing state must win over Default; got Selected=%q", got)
	}
}

func TestSanitize_FacetFieldExplicitUnsetCountsAsExisting(t *testing.T) {
	// A record that's been touched and the user cleared the facet ends
	// up with {set:false, selected:""}. That is an explicit state and
	// Default must NOT re-seed over it.
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN"},
	}
	raw := map[string]any{
		"title": "Hello",
		"_meta": map[string]any{
			"facets": map[string]any{
				"status": map[string]any{"set": false, "selected": ""},
			},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if got := out.Meta.Facets["status"]; got.Set || got.Selected != "" {
		t.Errorf("explicit unset must be preserved; got %+v", got)
	}
}

func TestSanitize_FacetFieldNoDefaultMeansNoSeed(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio"},
	}
	out := Sanitize(map[string]any{"title": "Hello"}, fields, SanitizeOptions{})
	if _, ok := out.Meta.Facets["status"]; ok {
		t.Errorf("no Default should seed nothing; got Facets=%+v", out.Meta.Facets)
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

// ── ExpressionItem harvest for virtual facet field ──────────────────

func TestExtendedLoadForm_FacetExpressionItemHarvestedFromMeta(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml", ItemField: "title",
		Facets: []template.Facet{{
			Key:  "status",
			Icon: "fa-flag",
			Options: []template.FacetOption{
				{Label: "OPEN", Color: "blue"},
				{Label: "CLOSED", Color: "gray"},
			},
		}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN", ExpressionItem: true},
		},
	})
	_ = sys.SaveFile("storage/basic/x.meta.json",
		`{"meta":{"id":"abc","template":"basic","facets":{"status":{"set":true,"selected":"OPEN"}}},"data":{"title":"Hello"}}`)

	got, err := m.ExtendedLoadForm("basic.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("ExtendedLoadForm: %v", err)
	}
	if got == nil {
		t.Fatal("expected summary, got nil")
	}
	v, ok := got.ExpressionItems["status_inline"].(string)
	if !ok {
		t.Fatalf("ExpressionItems[status_inline] = %#v, want string", got.ExpressionItems["status_inline"])
	}
	if v != "OPEN" {
		t.Errorf("ExpressionItems[status_inline] = %q, want %q (harvested from meta.facets[status].selected)", v, "OPEN")
	}
}

func TestExtendedLoadForm_UnsetFacetExpressionItemOmitted(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml", ItemField: "title",
		Facets: []template.Facet{{
			Key:     "status",
			Icon:    "fa-flag",
			Options: []template.FacetOption{{Label: "OPEN", Color: "blue"}},
		}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN", ExpressionItem: true},
		},
	})
	// Facet explicitly cleared ({set:false}), so the field's Default must not
	// re-seed it: the expression item stays genuinely unset and omitted.
	_ = sys.SaveFile("storage/basic/x.meta.json",
		`{"meta":{"id":"abc","template":"basic","facets":{"status":{"set":false}}},"data":{"title":"Hello"}}`)

	got, err := m.ExtendedLoadForm("basic.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("ExtendedLoadForm: %v", err)
	}
	if _, present := got.ExpressionItems["status_inline"]; present {
		t.Errorf("unset facet must not appear in ExpressionItems; got %+v", got.ExpressionItems)
	}
}
