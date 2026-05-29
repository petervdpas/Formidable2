package query

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func sourceByID(srcs []SourceInfo, id string) (SourceInfo, bool) {
	for _, s := range srcs {
		if s.ID == id {
			return s, true
		}
	}
	return SourceInfo{}, false
}

func TestDeriveSources_FieldsTableColumnsAndFacets(t *testing.T) {
	tpl := &template.Template{
		Facets: []template.Facet{{Key: "tier", Options: []template.FacetOption{{Label: "gold"}, {Label: "silver"}}}},
		Fields: []template.Field{
			{Key: "team", Type: "text", Label: "Team"},
			{Key: "score", Type: "number"},
			{Key: "due", Type: "date"},
			{Key: "status", Type: "dropdown", Options: []any{
				map[string]any{"value": "open", "label": "Open"},
				map[string]any{"value": "done", "label": "Done"},
			}},
			{Key: "tags", Type: "tags"},
			tableField("apps", [2]string{"name", "string"}, [2]string{"cost", "number"}),
			{Key: "avatar", Type: "image"}, // skipped
		},
	}
	srcs := deriveSources(tpl)

	// scalar text
	if s, ok := sourceByID(srcs, sourceID(scalar("team"))); !ok || s.Label != "Team" || s.Numeric || s.Fans {
		t.Fatalf("team source wrong: %+v ok=%v", s, ok)
	}
	// numeric scalar is aggregatable
	if s, _ := sourceByID(srcs, sourceID(scalar("score"))); !s.Numeric || !s.Aggregatable {
		t.Fatalf("score should be numeric+aggregatable: %+v", s)
	}
	// date
	if s, _ := sourceByID(srcs, sourceID(scalar("due"))); !s.Date {
		t.Fatalf("due should be date: %+v", s)
	}
	// dropdown carries choices
	if s, _ := sourceByID(srcs, sourceID(scalar("status"))); len(s.Choices) != 2 || s.Choices[0].Value != "open" {
		t.Fatalf("status choices wrong: %+v", s.Choices)
	}
	// tags fans
	if s, _ := sourceByID(srcs, sourceID(scalar("tags"))); !s.Fans {
		t.Fatalf("tags should fan: %+v", s)
	}
	// table columns: two, name (text) and cost (number, aggregatable), both fan
	if s, ok := sourceByID(srcs, sourceID(tcol("apps", 0))); !ok || !s.Fans || s.Numeric || s.Label != "apps / name" {
		t.Fatalf("apps name col wrong: %+v ok=%v", s, ok)
	}
	if s, _ := sourceByID(srcs, sourceID(tcol("apps", 1))); !s.Fans || !s.Numeric || !s.Aggregatable {
		t.Fatalf("apps cost col should be numeric+aggregatable+fans: %+v", s)
	}
	// facet with choices, does not fan
	if s, ok := sourceByID(srcs, sourceID(facet("tier"))); !ok || s.Fans || len(s.Choices) != 2 {
		t.Fatalf("tier facet wrong: %+v ok=%v", s, ok)
	}
	// image skipped
	if _, ok := sourceByID(srcs, sourceID(scalar("avatar"))); ok {
		t.Fatal("image field should be skipped")
	}
}

func TestManagerSources_LoadsTemplate(t *testing.T) {
	srcs, err := NewManager(fakeLoader{tpl: appsTpl()}).Sources("t.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sourceByID(srcs, sourceID(scalar("team"))); !ok {
		t.Fatal("expected team source from manager")
	}
}

func TestManagerSources_MissingTemplateErrors(t *testing.T) {
	if _, err := NewManager(fakeLoader{tpl: nil}).Sources("x.yaml"); err == nil {
		t.Fatal("missing template should error")
	}
}
