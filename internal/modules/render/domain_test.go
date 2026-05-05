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
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{form: form}, nil, nil)

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
	m := NewManager(&fakeTemplateLoader{err: errors.New("boom")}, &fakeFormStore{}, nil, nil)
	_, err := m.RenderForm("tpl", "data")
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestManager_RenderForm_MissingForm(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `{{x}}`}
	// nil form → render with empty values (mirror form.Manager.BuildView).
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{form: nil}, nil, nil)
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
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{}, nil, nil)
	got, err := m.RenderMarkdown("tpl", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "static" {
		t.Errorf("got %q", got)
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
		nil,
	)
	res, err := m.RenderForm("tpl.yaml", "df")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Markdown != "/storage/tpl.yaml/images/p.png" {
		t.Errorf("got %q", res.Markdown)
	}
}
