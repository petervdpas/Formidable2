package storage

import (
	"reflect"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestSanitize_FacetsFromInjectedMeta(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "Hello",
		"_meta": map[string]any{
			"facets": map[string]any{
				"flag": map[string]any{"set": true, "selected": "FLASH"},
			},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	want := map[string]FacetState{"flag": {Set: true, Selected: "FLASH"}}
	if !reflect.DeepEqual(out.Meta.Facets, want) {
		t.Errorf("Facets = %+v, want %+v", out.Meta.Facets, want)
	}
}

func TestSanitize_FacetsFromRawMetaEnvelope(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"data": map[string]any{"title": "Hi"},
		"meta": map[string]any{
			"facets": map[string]any{
				"status": map[string]any{"set": true, "selected": "IN USE"},
			},
		},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{})
	want := map[string]FacetState{"status": {Set: true, Selected: "IN USE"}}
	if !reflect.DeepEqual(out.Meta.Facets, want) {
		t.Errorf("Facets = %+v, want %+v", out.Meta.Facets, want)
	}
}

func TestSanitize_FacetsFromOptions(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	opts := SanitizeOptions{Facets: map[string]FacetState{"status": {Set: true, Selected: "PRIORITY"}}}
	out := Sanitize(map[string]any{"title": "X"}, fields, opts)
	if got := out.Meta.Facets["status"]; got != (FacetState{Set: true, Selected: "PRIORITY"}) {
		t.Errorf("Facets[status] = %+v, want PRIORITY/Set:true", got)
	}
}

func TestSanitize_OptionsWinsOverRawMeta(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"data": map[string]any{"title": "X"},
		"meta": map[string]any{
			"facets": map[string]any{
				"flag": map[string]any{"set": true, "selected": "FLASH"},
			},
		},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{
		Facets: map[string]FacetState{"flag": {Set: true, Selected: "ROUTINE"}},
	})
	if got := out.Meta.Facets["flag"].Selected; got != "ROUTINE" {
		t.Errorf("Selected = %q, want opts to win (ROUTINE)", got)
	}
}

func TestSanitize_FacetsDefaultsNil(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{})
	if out.Meta.Facets != nil {
		t.Errorf("Facets = %+v, want nil (no state, no map)", out.Meta.Facets)
	}
}

func TestSanitize_LegacyFlaggedTrueMigratesToFacet(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "Hello",
		"_meta": map[string]any{"flagged": true},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	got := out.Meta.Facets["flag"]
	if !got.Set {
		t.Errorf("legacy flagged=true should migrate to facets.flag.set=true; got %+v", got)
	}
	if got.Selected != "" {
		t.Errorf("Selected = %q, want empty (legacy bool carries no label)", got.Selected)
	}
}

func TestSanitize_LegacyFlaggedAndStateMigrate(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "X",
		"_meta": map[string]any{
			"flagged":    true,
			"flag_state": "FLASH",
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	got := out.Meta.Facets["flag"]
	if !got.Set || got.Selected != "FLASH" {
		t.Errorf("legacy pair = %+v, want Set:true Selected:FLASH", got)
	}
}

func TestSanitize_LegacyStateAloneMigratesAsUnset(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "X",
		"_meta": map[string]any{"flag_state": "FLASH"},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	got := out.Meta.Facets["flag"]
	if got.Set {
		t.Errorf("Set = true, want false (legacy state without flagged stays unset)")
	}
	if got.Selected != "FLASH" {
		t.Errorf("Selected = %q, want FLASH", got.Selected)
	}
}

func TestSanitize_FacetEntryNonObjectIgnored(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "X",
		"_meta": map[string]any{
			"facets": map[string]any{
				"bogus": "not-an-object",
				"flag":  map[string]any{"set": true},
			},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if _, ok := out.Meta.Facets["bogus"]; ok {
		t.Errorf("non-object facet entry should be skipped")
	}
	if !out.Meta.Facets["flag"].Set {
		t.Errorf("valid neighbour should still parse")
	}
}
