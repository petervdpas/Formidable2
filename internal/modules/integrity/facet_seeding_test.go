package integrity

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// LoadFormRaw makes the stub a RawFormReader; in production it reflects disk
// without seeding, here it just returns the test's form verbatim.
func (s *stubStorage) LoadFormRaw(tpl, fn string) *storage.Form {
	return s.forms[tpl][fn]
}

// A form whose disk is missing a facet the template defaults must be flagged,
// even though a sanitized load would seed it and hide the gap.
func TestAnalyzeTemplate_FlagsFormMissingDefaultedFacetOnDisk(t *testing.T) {
	tpl := &template.Template{
		Name: "gaps", Filename: "gaps.yaml",
		Facets: []template.Facet{{Key: "status", Icon: "fa-flag",
			Options: []template.FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status-field", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN"},
		},
	}
	forms := map[string]*storage.Form{
		"missing.meta.json": {Meta: storage.FormMeta{ID: "m"}},
		"present.meta.json": {Meta: storage.FormMeta{ID: "p",
			Facets: map[string]storage.FacetState{"status": {Set: true, Selected: "OPEN"}}}},
		"cleared.meta.json": {Meta: storage.FormMeta{ID: "c",
			Facets: map[string]storage.FacetState{"status": {Set: false}}}},
	}
	m := newM(t, tpl, forms)

	r, err := m.AnalyzeTemplate("gaps.yaml")
	if err != nil {
		t.Fatal(err)
	}

	flagged := map[string]string{}
	for _, fr := range r.Forms {
		for _, iss := range fr.Issues {
			if iss.Kind == IssueFacetUnseeded {
				flagged[fr.Filename] = iss.Suggest
			}
		}
	}
	if flagged["missing.meta.json"] != "OPEN" {
		t.Errorf("missing form must be flagged IssueFacetUnseeded suggesting OPEN; report=%+v", r.Forms)
	}
	if _, ok := flagged["present.meta.json"]; ok {
		t.Error("a form that already has the facet must not be flagged")
	}
	if _, ok := flagged["cleared.meta.json"]; ok {
		t.Error("an explicitly cleared facet must not be flagged")
	}
}

// Repair seeds the template default onto a form whose disk is missing the facet.
func TestFix_SeedFacet_WritesDefaultOntoMissingFacet(t *testing.T) {
	tpl := &template.Template{
		Name: "gaps", Filename: "gaps.yaml",
		Facets: []template.Facet{{Key: "status", Icon: "fa-flag",
			Options: []template.FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status-field", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN"},
		},
	}
	h := newFixHarness(t, tpl, map[string]*storage.Form{
		"a.meta.json": {Meta: storage.FormMeta{ID: "a"}},
	})

	res := h.runPlan(FixPlanItem{Kind: IssueFacetUnseeded, Strategy: FixSeedFacet})

	if res.Applied != 1 || res.FormsSaved != 1 {
		t.Fatalf("Applied=%d FormsSaved=%d; want 1/1: %+v", res.Applied, res.FormsSaved, res)
	}
	got := h.loadSaved("a.meta.json").Meta.Facets["status"]
	if !got.Set || got.Selected != "OPEN" {
		t.Errorf("facet not seeded with default: %+v", got)
	}
}
