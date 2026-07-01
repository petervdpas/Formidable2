package wiki

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/render"
	tpl "github.com/petervdpas/formidable2/internal/modules/template"
)

// fakeDeckProvider is a hand-shaped DeckProvider for the presentation routes.
type fakeDeckProvider struct {
	decks     []DeckList
	orderFor  map[string][]string // deck value -> datafiles
	sequence  []string
	lastBuild []string // datafiles passed to the last BuildDeck call
}

func (f *fakeDeckProvider) Decks(string) ([]DeckList, error) { return f.decks, nil }
func (f *fakeDeckProvider) DeckOrder(_ string, deck string) ([]string, error) {
	return f.orderFor[deck], nil
}
func (f *fakeDeckProvider) SequenceOrder(string) ([]string, error) { return f.sequence, nil }
func (f *fakeDeckProvider) BuildDeck(_ string, datafiles []string) (render.RevealDeck, error) {
	f.lastBuild = datafiles
	return render.RevealDeck{
		HTML:   "<section><div class=\"slide-canvas\">SLIDE-BODY</div></section>",
		Width:  1280,
		Height: 720,
	}, nil
}

// newDeckHandler builds a handler whose "talk.yaml" is a presentation with two
// decks; "basic.yaml" stays a normal template.
func newDeckHandler(t *testing.T, decks *fakeDeckProvider) http.Handler {
	t.Helper()
	sp := newStubProvider()
	sp.templates = append(sp.templates, dataprovider.TemplateSummary{
		Stem: "talk", Filename: "talk.yaml", Name: "Talk 2026",
	})
	st := &stubTemplates{byName: map[string]*tpl.Template{
		"talk.yaml":  {Filename: "talk.yaml", Presentation: true},
		"basic.yaml": {Filename: "basic.yaml"},
	}}
	h := NewHandler(sp, newStubStorage(), &stubExpressioner{})
	h.SetTemplates(st)
	h.SetDecks(decks)
	return h
}

func twoDecks() *fakeDeckProvider {
	return &fakeDeckProvider{
		decks: []DeckList{{Value: "intro", Label: "Intro"}, {Value: "deep", Label: "Deep dive"}},
		orderFor: map[string][]string{
			"intro": {"s1.meta.json", "s2.meta.json"},
			"deep":  {"s3.meta.json"},
		},
	}
}

func TestIndex_PresentationsSeparateFromTemplates(t *testing.T) {
	h := newDeckHandler(t, twoDecks())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	body := w.Body.String()
	if !strings.Contains(body, "Presentations") {
		t.Error("index missing Presentations heading")
	}
	if !strings.Contains(body, `href="/template/talk/slides/intro"`) ||
		!strings.Contains(body, `href="/template/talk/slides/deep"`) {
		t.Errorf("index missing per-deck play links; body:\n%s", body)
	}
	// The presentation must NOT appear in the normal template list (that link has
	// no /slides suffix).
	if strings.Contains(body, `href="/template/talk"`) {
		t.Error("presentation template leaked into the normal Templates list")
	}
	// A normal template is still listed normally.
	if !strings.Contains(body, `href="/template/basic"`) {
		t.Error("normal template missing from Templates list")
	}
}

func TestDeck_RendersRevealPage(t *testing.T) {
	h := newDeckHandler(t, twoDecks())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/talk/slides", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`class="reveal"`, `data-width="1280"`, `data-height="720"`,
		"SLIDE-BODY",
		"/_/css/reveal.css", "/_/katex/katex.min.css", "/_/css/deck.css",
		"/_/js/reveal.js", "/_/js/deck-init.js",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("deck page missing %q", want)
		}
	}
}

func TestDeck_EmptyDeckPlaysFirst(t *testing.T) {
	d := twoDecks()
	h := newDeckHandler(t, d)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/talk/slides", nil))

	if got := strings.Join(d.lastBuild, ","); got != "s1.meta.json,s2.meta.json" {
		t.Errorf("empty deck built %q, want the first deck (intro)", got)
	}
}

func TestDeck_SpecificDeckUsesDeckOrder(t *testing.T) {
	d := twoDecks()
	h := newDeckHandler(t, d)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/talk/slides/deep", nil))

	if got := strings.Join(d.lastBuild, ","); got != "s3.meta.json" {
		t.Errorf("deck 'deep' built %q, want s3.meta.json", got)
	}
}

func TestDeck_SingleDeckUsesSequence(t *testing.T) {
	d := &fakeDeckProvider{sequence: []string{"a.meta.json", "b.meta.json"}}
	h := newDeckHandler(t, d)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/talk/slides", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := strings.Join(d.lastBuild, ","); got != "a.meta.json,b.meta.json" {
		t.Errorf("single-deck built %q, want the whole sequence", got)
	}
}

func TestDeck_NonPresentation404(t *testing.T) {
	h := newDeckHandler(t, twoDecks())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic/slides", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for non-presentation", w.Code)
	}
}

func TestDeck_NoProvider404(t *testing.T) {
	sp := newStubProvider()
	st := &stubTemplates{byName: map[string]*tpl.Template{"basic.yaml": {Filename: "basic.yaml", Presentation: true}}}
	h := NewHandler(sp, newStubStorage(), &stubExpressioner{})
	h.SetTemplates(st)
	// No SetDecks: nothing to play.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic/slides", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no deck provider wired", w.Code)
	}
}

func TestStatic_DeckAssets(t *testing.T) {
	h := newDeckHandler(t, twoDecks())
	cases := []struct {
		path string
		mime string
	}{
		{"/_/js/reveal.js", "text/javascript; charset=utf-8"},
		{"/_/css/reveal.css", "text/css; charset=utf-8"},
		{"/_/css/deck.css", "text/css; charset=utf-8"},
		{"/_/js/deck-init.js", "text/javascript; charset=utf-8"},
		{"/_/katex/katex.min.css", "text/css; charset=utf-8"},
		{"/_/katex/katex.min.js", "text/javascript; charset=utf-8"},
		{"/_/katex/fonts/KaTeX_Main-Regular.woff2", "font/woff2"},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", c.path, w.Code)
			continue
		}
		if w.Body.Len() == 0 {
			t.Errorf("%s: empty body", c.path)
		}
		if got := w.Header().Get("Content-Type"); got != c.mime {
			t.Errorf("%s: content-type = %q, want %q", c.path, got, c.mime)
		}
	}
}

func TestStatic_KatexTraversalBlocked(t *testing.T) {
	h := newDeckHandler(t, twoDecks())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/katex/../reveal.js", nil))
	// The mux cleans `..` and 301-redirects off the katex prefix; a raw `..` never
	// reaches KatexFS. Either way it must not serve content.
	if w.Code == http.StatusOK {
		t.Errorf("traversal served content (status 200); want redirect or 404")
	}
}
