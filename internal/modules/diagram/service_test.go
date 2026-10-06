package diagram

import (
	"errors"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

type fakeLoader struct {
	t   *template.Template
	err error
}

func (f fakeLoader) LoadTemplate(string) (*template.Template, error) { return f.t, f.err }

func TestService_RenderDrawsLiveData(t *testing.T) {
	s := NewService(fakeLoader{t: lineageTemplate()})
	svg, err := s.Render("m.yaml", "lineage", map[string]any{"mapping": []any{[]any{"A", "x", "T", "y", ""}}})
	if err != nil || !strings.HasPrefix(svg, "<svg") {
		t.Fatalf("svg=%.60q err=%v", svg, err)
	}
}

func TestService_RenderPropagatesLoadError(t *testing.T) {
	boom := errors.New("boom")
	if _, err := NewService(fakeLoader{err: boom}).Render("m.yaml", "lineage", nil); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestService_RenderRejectsNonDiagramField(t *testing.T) {
	if _, err := NewService(fakeLoader{t: lineageTemplate()}).Render("m.yaml", "mapping", nil); err == nil {
		t.Fatal("want error for a non-diagram field")
	}
}
