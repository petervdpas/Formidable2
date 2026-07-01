package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

type keyedFormStore struct{ forms map[string]*storage.Form }

func (k *keyedFormStore) LoadForm(_, df string) *storage.Form { return k.forms[df] }

func TestBuildDeck_SectionsWithRevealAttrs(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{
		{Key: "slide", Type: "slide", Options: []any{
			map[string]any{"value": "canvas_width", "label": "1920"},
			map[string]any{"value": "canvas_height", "label": "1080"},
		}},
	}}
	doc1 := map[string]any{
		"blocks": []any{map[string]any{
			"id": "b1", "kind": "text", "content": "Hi", "x": 10, "y": 20, "w": 300, "h": 100,
		}},
		"background": "#ff0000",
		"transition": "fade",
		"notes":      "hello notes",
	}
	doc2 := map[string]any{
		"blocks": []any{map[string]any{
			"id": "b2", "kind": "text", "content": "Bye", "x": 0, "y": 0, "w": 100, "h": 50,
		}},
	}
	store := &keyedFormStore{forms: map[string]*storage.Form{
		"a.meta.json": {Data: map[string]any{"slide": doc1}},
		"b.meta.json": {Data: map[string]any{"slide": doc2}},
	}}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, store, nil, nil, nil)

	deck, err := m.BuildDeck("tpl", []string{"a.meta.json", "b.meta.json"})
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	if deck.Width != 1920 || deck.Height != 1080 {
		t.Errorf("size = %dx%d, want 1920x1080", deck.Width, deck.Height)
	}
	if n := strings.Count(deck.HTML, "<section"); n != 2 {
		t.Errorf("want 2 sections, got %d in %q", n, deck.HTML)
	}
	if !strings.Contains(deck.HTML, `data-background-color="#ff0000"`) {
		t.Errorf("missing background attr: %q", deck.HTML)
	}
	if !strings.Contains(deck.HTML, `data-transition="fade"`) {
		t.Errorf("missing transition attr: %q", deck.HTML)
	}
	if !strings.Contains(deck.HTML, `class="notes"`) {
		t.Errorf("missing speaker-notes aside: %q", deck.HTML)
	}
	if !strings.Contains(deck.HTML, "slide-canvas") {
		t.Errorf("missing positioned canvas: %q", deck.HTML)
	}
}

func TestBuildDeck_NoSlideField(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{{Key: "id", Type: "guid"}}}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{}, nil, nil, nil)
	if _, err := m.BuildDeck("tpl", []string{"a"}); err == nil {
		t.Errorf("expected an error when the template has no slide field")
	}
}
