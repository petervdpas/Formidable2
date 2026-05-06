package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

type fakeTemplateLoader struct {
	tpl *template.Template
	err error
}

func (f *fakeTemplateLoader) LoadTemplate(_ string) (*template.Template, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tpl, nil
}

type fakeFormStore struct {
	form *storage.Form
}

func (f *fakeFormStore) LoadForm(_, _ string) *storage.Form {
	return f.form
}

func TestManager_RenderForm(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `# {{title}}`,
		Fields: []template.Field{
			{Key: "title", Type: "text"},
		},
	}
	form := &storage.Form{Data: map[string]any{"title": "Hello"}}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{form: form}, nil, nil, nil)

	res, err := m.RenderForm("tpl", "data")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Markdown != "# Hello" {
		t.Errorf("markdown = %q", res.Markdown)
	}
	if !strings.Contains(res.HTML, "<h1") {
		t.Errorf("html = %q", res.HTML)
	}
}

func TestManager_RenderForm_TemplateError(t *testing.T) {
	m := NewManager(&fakeTemplateLoader{err: errors.New("boom")}, &fakeFormStore{}, nil, nil, nil)
	_, err := m.RenderForm("tpl", "data")
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestManager_RenderForm_MissingForm(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `{{x}}`}
	// nil form → render with empty values (mirror form.Manager.BuildView).
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{form: nil}, nil, nil, nil)
	res, err := m.RenderForm("tpl", "data")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Markdown != "" {
		t.Errorf("expected empty render, got %q", res.Markdown)
	}
}

func TestManager_RenderMarkdown_NoDatafile(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `static`}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{}, nil, nil, nil)
	got, err := m.RenderMarkdown("tpl", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "static" {
		t.Errorf("got %q", got)
	}
}

func TestManager_FormidableLinkURLStrategy_Wiki(t *testing.T) {
	// Wiki context: the FormidableLinkURL strategy rewrites
	// formidable://<tpl>:<df> hrefs at the source. Goldmark sees
	// already-rewritten URLs and emits plain `<a href="/template/.../">`
	// — no post-process regex required at the wiki handler.
	tpl := &template.Template{
		MarkdownTemplate: `{{field "ref"}}`,
		Fields:           []template.Field{{Key: "ref", Type: "link"}},
	}
	form := &storage.Form{Data: map[string]any{
		"ref": map[string]any{
			"href": "formidable://recepten.yaml:groene-tapenade.meta.json",
			"text": "Tapenade",
		},
	}}
	m := NewManager(
		&fakeTemplateLoader{tpl: tpl},
		&fakeFormStore{form: form},
		nil,
		func(templateFilename, datafile string) string {
			stem := strings.TrimSuffix(templateFilename, ".yaml")
			return "/template/" + stem + "/form/" + datafile
		},
		nil, // log
	)
	res, err := m.RenderForm("tpl.yaml", "df")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "[Tapenade](/template/recepten/form/groene-tapenade.meta.json)"
	if res.Markdown != want {
		t.Errorf("markdown = %q, want %q", res.Markdown, want)
	}
}

func TestManager_FormidableLinkURLStrategy_Slideout(t *testing.T) {
	// Slideout context: no FormidableLinkURL → formidable:// URLs
	// pass through unchanged, the Vue click interceptor handles
	// them in-app via the Nav service.
	tpl := &template.Template{
		MarkdownTemplate: `{{field "ref"}}`,
		Fields:           []template.Field{{Key: "ref", Type: "link"}},
	}
	form := &storage.Form{Data: map[string]any{
		"ref": map[string]any{
			"href": "formidable://recepten.yaml:groene-tapenade.meta.json",
			"text": "Tapenade",
		},
	}}
	m := NewManager(
		&fakeTemplateLoader{tpl: tpl},
		&fakeFormStore{form: form},
		nil,
		nil, // formidable link URL — passthrough
		nil, // log
	)
	res, err := m.RenderForm("tpl.yaml", "df")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "[Tapenade](formidable://recepten.yaml:groene-tapenade.meta.json)"
	if res.Markdown != want {
		t.Errorf("markdown = %q, want %q", res.Markdown, want)
	}
}

func TestManager_FormidableLinkURLStrategy_MalformedFallsBack(t *testing.T) {
	// A formidable:// URL with no `:` separator can't be parsed;
	// resolveLinkHref must keep the original href and not call the
	// strategy with empty parts (which could yield a malformed URL).
	tpl := &template.Template{
		MarkdownTemplate: `{{field "ref"}}`,
		Fields:           []template.Field{{Key: "ref", Type: "link"}},
	}
	form := &storage.Form{Data: map[string]any{
		"ref": map[string]any{
			"href": "formidable://no-colon-here",
			"text": "broken",
		},
	}}
	m := NewManager(
		&fakeTemplateLoader{tpl: tpl},
		&fakeFormStore{form: form},
		nil,
		func(templateFilename, datafile string) string {
			t.Errorf("strategy should not be called for malformed URLs (tpl=%q df=%q)", templateFilename, datafile)
			return "/should/not/appear"
		},
		nil,
	)
	res, err := m.RenderForm("tpl.yaml", "df")
	if err != nil {
		t.Fatal(err)
	}
	want := "[broken](formidable://no-colon-here)"
	if res.Markdown != want {
		t.Errorf("markdown = %q, want %q", res.Markdown, want)
	}
}

func TestManager_ImagePathStrategy(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `{{field "logo"}}`,
		Fields:           []template.Field{{Key: "logo", Type: "image"}},
	}
	form := &storage.Form{Data: map[string]any{"logo": "p.png"}}
	m := NewManager(
		&fakeTemplateLoader{tpl: tpl},
		&fakeFormStore{form: form},
		func(templateFilename, name string) string {
			return "/storage/" + templateFilename + "/images/" + name
		},
		nil, // formidable link URL
		nil, // log
	)
	res, err := m.RenderForm("tpl.yaml", "df")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Markdown != "/storage/tpl.yaml/images/p.png" {
		t.Errorf("got %q", res.Markdown)
	}
}

func TestManager_DesktopFileURLStrategy(t *testing.T) {
	// Desktop wiring (composition root in internal/app/app.go) builds
	// a `file://<abs>/storage/<tpl>/images/<file>` URL from the
	// storage manager's TemplateImageDir + filename. Mirror that
	// shape with a realistic markdown template that wraps the URL in
	// an `![alt](url)` image so the HTML stage's convertFileImages
	// pass actually fires.
	tpl := &template.Template{
		MarkdownTemplate: `![logo]({{field "logo"}})`,
		Fields:           []template.Field{{Key: "logo", Type: "image"}},
	}
	form := &storage.Form{Data: map[string]any{"logo": "icon.png"}}
	m := NewManager(
		&fakeTemplateLoader{tpl: tpl},
		&fakeFormStore{form: form},
		func(templateFilename, name string) string {
			return "file:///abs/storage/" + templateFilename + "/images/" + name
		},
		nil, // formidable link URL
		nil, // log
	)
	res, err := m.RenderForm("recepten.yaml", "df")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	wantMD := "![logo](file:///abs/storage/recepten.yaml/images/icon.png)"
	if res.Markdown != wantMD {
		t.Errorf("markdown got %q, want %q", res.Markdown, wantMD)
	}
	// HTML stage's pre-rewrite must keep file:// images intact —
	// they're inlined as raw <img src="…"> by convertFileImages.
	wantSrc := `src="file:///abs/storage/recepten.yaml/images/icon.png"`
	if !strings.Contains(res.HTML, wantSrc) {
		t.Errorf("HTML lost the file:// src: %q", res.HTML)
	}
}

func TestManager_RenderFullHTML(t *testing.T) {
	// Markdown carries a frontmatter title that should land in <title>;
	// the rendered fragment lands in <body class="formidable-prose">.
	tpl := &template.Template{
		MarkdownTemplate: "---\ntitle: Spaanse Groentenschotel\n---\n# Body\n",
	}
	form := &storage.Form{Data: map[string]any{}}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{form: form}, nil, nil, nil)

	out, err := m.RenderFullHTML("tpl", "df.meta.json")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"<!DOCTYPE html>",
		`<html lang="en">`,
		`<meta charset="utf-8">`,
		"<title>Spaanse Groentenschotel</title>",
		"<style>",                          // inlined CSS
		".formidable-prose",                // a known selector from the embedded sheet
		`<body class="formidable-prose">`,
		"</body>",
		"</html>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull:\n%s", want, out)
		}
	}
	// The H1 from the markdown body must be in the document.
	if !strings.Contains(out, "<h1") {
		t.Errorf("expected rendered <h1>, got: %s", out)
	}
}

func TestManager_RenderFullHTML_TitleFallback(t *testing.T) {
	// No frontmatter title → fall back to the datafile stem.
	tpl := &template.Template{MarkdownTemplate: "# only body\n"}
	form := &storage.Form{Data: map[string]any{}}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{form: form}, nil, nil, nil)

	out, err := m.RenderFullHTML("tpl", "groene-tapenade.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<title>groene-tapenade</title>") {
		t.Errorf("title fallback wrong; got: %s", out)
	}
}

func TestManager_RenderFullHTML_FrontmatterIsStrippedFromBody(t *testing.T) {
	// The frontmatter line should appear only in <title>, not as text
	// in the body (RenderHTML already strips it; this guards regression).
	tpl := &template.Template{MarkdownTemplate: "---\ntitle: My Doc\n---\n# heading\n"}
	form := &storage.Form{Data: map[string]any{}}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{form: form}, nil, nil, nil)

	out, err := m.RenderFullHTML("tpl", "df.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	// Title in <title>, fine. But the literal "title: My Doc" line
	// shouldn't leak into the body output.
	bodyIdx := strings.Index(out, "<body")
	if bodyIdx < 0 {
		t.Fatal("no body")
	}
	if strings.Contains(out[bodyIdx:], "title: My Doc") {
		t.Errorf("frontmatter leaked into body: %s", out)
	}
}

func TestManager_NoStrategyFallsBackToRelativeImagesPath(t *testing.T) {
	// nil ImageURLFunc — emitter falls back to `images/<name>`. This
	// keeps RenderMarkdown usable for export tooling that has no
	// per-template URL strategy.
	tpl := &template.Template{
		MarkdownTemplate: `![logo]({{field "logo"}})`,
		Fields:           []template.Field{{Key: "logo", Type: "image"}},
	}
	form := &storage.Form{Data: map[string]any{"logo": "icon.png"}}
	m := NewManager(
		&fakeTemplateLoader{tpl: tpl},
		&fakeFormStore{form: form},
		nil, // image URL
		nil, // formidable link URL
		nil, // log
	)
	res, err := m.RenderForm("tpl.yaml", "df")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if want := "![logo](images/icon.png)"; res.Markdown != want {
		t.Errorf("markdown got %q, want %q", res.Markdown, want)
	}
	// `images/<name>` (relative under the same template) also gets
	// rewritten by convertFileImages so help-asset-style templates
	// render properly without a server prefix.
	if !strings.Contains(res.HTML, `src="images/icon.png"`) {
		t.Errorf("HTML lost relative img src: %q", res.HTML)
	}
}
