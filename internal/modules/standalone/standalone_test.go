package standalone

import (
	"errors"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// --- fakes ---------------------------------------------------------------

type fakeTemplates struct{ tpl *template.Template }

func (f fakeTemplates) LoadTemplate(string) (*template.Template, error) { return f.tpl, nil }

type fakeForms struct{ forms map[string]*storage.Form }

func (f fakeForms) LoadForm(_, df string) *storage.Form { return f.forms[df] }

type fakeSource struct {
	plan     Plan
	err      error
	lastDeck string
}

func (f *fakeSource) Plan(_, deck string) (Plan, error) {
	f.lastDeck = deck
	return f.plan, f.err
}

// realRenderer builds a render.Manager whose image strategy inlines a fixed
// data URL, so exports assert images arrive as data URIs (not server paths).
func realRenderer(tpl *template.Template, forms map[string]*storage.Form) *render.Manager {
	dataURL := func(_, _ string) string { return "data:image/png;base64,Zm9v" }
	m := render.NewManager(fakeTemplates{tpl: tpl}, fakeForms{forms: forms}, dataURL, nil, nil)
	m.SetImageBase64URL(dataURL)
	return m
}

// --- document ------------------------------------------------------------

func TestExport_Document_SelfContained(t *testing.T) {
	tpl := &template.Template{
		Name:             "Recipes",
		MarkdownTemplate: "# {{title}}\n\n{{body}}\n",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "body", Type: "textarea"},
		},
	}
	forms := map[string]*storage.Form{
		"a.meta.json": {Data: map[string]any{"title": "Alpha", "body": "first"}},
		"b.meta.json": {Data: map[string]any{"title": "Beta", "body": "second"}},
	}
	src := &fakeSource{plan: Plan{Title: "Recipes", Datafiles: []string{"a.meta.json", "b.meta.json"}}}

	out, err := NewService(realRenderer(tpl, forms), src).Export("recipes.yaml", "")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.HasPrefix(out, "<!DOCTYPE html>") {
		t.Errorf("missing doctype")
	}
	if !strings.Contains(out, "<title>Recipes</title>") {
		t.Errorf("title should come from plan: %q", out[:200])
	}
	if strings.Contains(out, "<link ") {
		t.Errorf("standalone doc must not reference external stylesheets")
	}
	if !strings.Contains(out, "formidable-prose") {
		t.Errorf("prose stylesheet not inlined")
	}
	ia, ib := strings.Index(out, "Alpha"), strings.Index(out, "Beta")
	if ia < 0 || ib < 0 || ia > ib {
		t.Errorf("records missing or out of order: Alpha@%d Beta@%d", ia, ib)
	}
	if n := strings.Count(out, `class="formidable-doc-record"`); n != 2 {
		t.Errorf("want 2 record articles, got %d", n)
	}
	if strings.Contains(out, "<script") {
		t.Errorf("plain doc should not inline scripts")
	}
}

func TestExport_Document_InlinesMermaid(t *testing.T) {
	tpl := &template.Template{
		Name:             "Diagrams",
		MarkdownTemplate: "```mermaid\ngraph TD; A-->B\n```\n",
		Fields:           []template.Field{{Key: "id", Type: "guid"}},
	}
	forms := map[string]*storage.Form{"a.meta.json": {Data: map[string]any{}}}
	src := &fakeSource{plan: Plan{Title: "Diagrams", Datafiles: []string{"a.meta.json"}}}

	out, err := NewService(realRenderer(tpl, forms), src).Export("diagrams.yaml", "")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(out, `class="mermaid"`) {
		t.Fatalf("mermaid block not rendered")
	}
	if !strings.Contains(out, "__esbuild_esm_mermaid_nm") {
		t.Errorf("vendored mermaid.min.js not inlined")
	}
}

func TestExport_Document_TitleFallsBackToStem(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: "x\n", Fields: []template.Field{{Key: "id", Type: "guid"}}}
	forms := map[string]*storage.Form{"a.meta.json": {Data: map[string]any{}}}
	src := &fakeSource{plan: Plan{Title: "", Datafiles: []string{"a.meta.json"}}}

	out, err := NewService(realRenderer(tpl, forms), src).Export("recipes.yaml", "")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(out, "<title>recipes</title>") {
		t.Errorf("title should fall back to filename stem")
	}
}

// --- deck ----------------------------------------------------------------

func deckTemplate() *template.Template {
	return &template.Template{
		Name:         "Deck",
		Presentation: true,
		Fields: []template.Field{
			{Key: "slide", Type: "slide", Options: []any{
				map[string]any{"value": "canvas_width", "label": "1920"},
				map[string]any{"value": "canvas_height", "label": "1080"},
			}},
		},
	}
}

func TestExport_Deck_SelfContained(t *testing.T) {
	tpl := deckTemplate()
	doc := map[string]any{"blocks": []any{map[string]any{
		"id": "b1", "kind": "text", "content": "Hi", "x": 10, "y": 20, "w": 300, "h": 100,
	}}}
	forms := map[string]*storage.Form{"a.meta.json": {Data: map[string]any{"slide": doc}}}
	src := &fakeSource{plan: Plan{Title: "Deck", Presentation: true, Datafiles: []string{"a.meta.json"}}}

	out, err := NewService(realRenderer(tpl, forms), src).Export("deck.yaml", "intro")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if src.lastDeck != "intro" {
		t.Errorf("deck selector not passed to source: got %q", src.lastDeck)
	}
	for _, want := range []string{
		"<!DOCTYPE html>", `class="reveal`, `data-width="1920"`, "slide-canvas", "Reveal",
		"<title>Deck - Formidable Slides</title>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("deck missing %q", want)
		}
	}
	if strings.Contains(out, "<link ") {
		t.Errorf("deck must not link external stylesheets")
	}
	if strings.Contains(out, `"/_/`) {
		t.Errorf("deck must not reference wiki /_/ asset paths")
	}
}

func TestExport_Deck_NoSlideFieldErrors(t *testing.T) {
	tpl := &template.Template{Name: "X", Presentation: true, Fields: []template.Field{{Key: "id", Type: "guid"}}}
	src := &fakeSource{plan: Plan{Title: "X", Presentation: true, Datafiles: []string{"a.meta.json"}}}
	if _, err := NewService(realRenderer(tpl, nil), src).Export("x.yaml", ""); err == nil {
		t.Errorf("expected BuildDeck error to propagate when template has no slide field")
	}
}

// --- source errors + katex ----------------------------------------------

func TestExport_PlanErrorPropagates(t *testing.T) {
	src := &fakeSource{err: errors.New("boom")}
	if _, err := NewService(realRenderer(&template.Template{}, nil), src).Export("x.yaml", ""); err == nil {
		t.Errorf("expected the plan error to propagate")
	}
}

func TestKatexInlineCSS_InlinesWoff2(t *testing.T) {
	css := katexInlineCSS()
	if css == "" {
		t.Fatal("katexInlineCSS returned empty")
	}
	if strings.Contains(css, "url(fonts/") {
		t.Errorf("relative font refs should be inlined/stripped")
	}
	if !strings.Contains(css, "data:font/woff2;base64,") {
		t.Errorf("woff2 fonts not inlined as data URIs")
	}
	if strings.Contains(css, `format("truetype")`) || strings.Contains(css, `format("woff")`) {
		t.Errorf("woff/ttf sources should be stripped")
	}
}
