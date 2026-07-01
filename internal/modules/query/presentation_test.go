package query

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// A presentation template holds slides, not queryable data, so every query entry
// point refuses it with ErrPresentationExcluded.
func TestQuery_PresentationTemplateExcluded(t *testing.T) {
	pres := &template.Template{
		Filename:     "talk.yaml",
		Presentation: true,
		Fields:       []template.Field{{Key: "x", Type: "text"}},
	}
	m := NewManager(fakeLoader{tpl: pres})

	if _, err := m.Sources("talk.yaml"); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("Sources err = %v, want ErrPresentationExcluded", err)
	}
	if _, err := m.Run(Spec{Template: "talk.yaml"}); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("Run err = %v, want ErrPresentationExcluded", err)
	}
	if _, err := m.Explain(Spec{Template: "talk.yaml"}); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("Explain err = %v, want ErrPresentationExcluded", err)
	}
}
