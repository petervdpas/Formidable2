package storage

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func facetSeedTemplate(t *testing.T, tplM *template.Manager) {
	t.Helper()
	if err := tplM.SaveTemplate("gaps.yaml", &template.Template{
		Name: "gaps", Filename: "gaps.yaml",
		Facets: []template.Facet{{Key: "status", Icon: "fa-flag",
			Options: []template.FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status-field", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

// LoadFormRaw must reflect the facets actually on disk, so the integrity doctor
// can see them; LoadForm would seed the field default and mask the truth.
func TestLoadFormRaw_CarriesDiskFacets(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	facetSeedTemplate(t, tplM)
	if err := sys.SaveFile("storage/gaps/f1.meta.json",
		`{"meta":{"id":"f1","template":"gaps","facets":{"status":{"set":true,"selected":"OPEN"}}},"data":{"title":"x"}}`); err != nil {
		t.Fatal(err)
	}

	raw := m.LoadFormRaw("gaps.yaml", "f1")
	if raw == nil {
		t.Fatal("nil raw form")
	}
	st, ok := raw.Meta.Facets["status"]
	if !ok || !st.Set || st.Selected != "OPEN" {
		t.Errorf("LoadFormRaw must carry disk facet status=OPEN, got %+v", raw.Meta.Facets)
	}
}

// A form whose disk has no facets must come back with none from LoadFormRaw
// (no seeding), even though LoadForm would seed the field default.
func TestLoadFormRaw_DoesNotSeedDefaults(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	facetSeedTemplate(t, tplM)
	if err := sys.SaveFile("storage/gaps/f1.meta.json",
		`{"meta":{"id":"f1","template":"gaps"},"data":{"title":"x"}}`); err != nil {
		t.Fatal(err)
	}

	raw := m.LoadFormRaw("gaps.yaml", "f1")
	if len(raw.Meta.Facets) != 0 {
		t.Errorf("LoadFormRaw must not seed (disk has no facets), got %+v", raw.Meta.Facets)
	}
	// Contrast: LoadForm seeds the default, which is exactly the masking.
	if san := m.LoadForm("gaps.yaml", "f1"); san.Meta.Facets["status"].Selected != "OPEN" {
		t.Errorf("precondition: LoadForm should seed default, got %+v", san.Meta.Facets)
	}
}
