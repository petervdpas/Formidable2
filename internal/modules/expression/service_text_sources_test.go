package expression

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestBuilderTextSources_DisplayableFieldsThenFormulas(t *testing.T) {
	s := NewService(nil)
	got := s.BuilderTextSources(
		[]template.Field{
			{Key: "code", Type: "text", Label: "Audit code"},
			{Key: "fac", Type: "facet", Label: "Status"}, // virtual -> not displayable -> excluded
			{Key: "naam", Type: "text"},                  // no label -> falls back to key
		},
		[]template.Formula{
			{Key: "reference", Type: "text", Label: "Reference"},
		},
	)

	if len(got) != 3 {
		t.Fatalf("got %d sources, want 3: %+v", len(got), got)
	}
	// displayable fields first, in order, grouped "field"
	if got[0].Key != "code" || got[0].Label != "Audit code" || got[0].Group != "field" {
		t.Errorf("source[0] = %+v", got[0])
	}
	if got[1].Key != "naam" || got[1].Label != "naam" || got[1].Group != "field" {
		t.Errorf("source[1] = %+v (label should fall back to key)", got[1])
	}
	// the virtual facet field must not appear
	for _, o := range got {
		if o.Key == "fac" {
			t.Errorf("non-displayable facet field leaked into sources: %+v", got)
		}
	}
	// formula last, grouped "formula"
	if got[2].Key != "reference" || got[2].Label != "Reference" || got[2].Group != "formula" {
		t.Errorf("source[2] = %+v, want the formula in group formula", got[2])
	}
}
