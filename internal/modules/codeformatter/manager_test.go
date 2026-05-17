package codeformatter

import (
	"errors"
	"strings"
	"testing"
)

func TestFormat_YAML_Roundtrip(t *testing.T) {
	in := `cover:
    template: classic
    title: Hello
toc:
    title: Contents
`
	out, err := NewManager(nil).Format("yaml", in)
	if err != nil {
		t.Fatal(err)
	}
	want := `cover:
  template: classic
  title: Hello
toc:
  title: Contents
`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestFormat_YAML_PreservesKeyOrder(t *testing.T) {
	in := "b: 2\na: 1\nc: 3\n"
	out, err := NewManager(nil).Format("yaml", in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("key order not preserved: got %q", out)
	}
}

func TestFormat_YAML_Empty(t *testing.T) {
	out, err := NewManager(nil).Format("yaml", "")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("got %q", out)
	}
}

func TestFormat_YAML_Malformed(t *testing.T) {
	in := "key: [unclosed\n"
	out, err := NewManager(nil).Format("yaml", in)
	if !errors.Is(err, ErrMalformed) {
		t.Errorf("expected ErrMalformed, got %v", err)
	}
	if out == "" {
		t.Errorf("expected tidy fallback content, got empty")
	}
}

func TestFormat_Markdown_FrontmatterOnly(t *testing.T) {
	in := `---
cover:
    template: classic
    title: Hello
---
`
	out, err := NewManager(nil).Format("markdown", in)
	if err != nil {
		t.Fatal(err)
	}
	want := `---
cover:
  template: classic
  title: Hello
---
`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestFormat_Markdown_FrontmatterAndBody(t *testing.T) {
	in := `---
cover:
    title: Hello
---

## {{field "name"}}

Body text with {{handlebars}} expressions.
`
	out, err := NewManager(nil).Format("markdown", in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cover:\n  title: Hello") {
		t.Errorf("frontmatter not reflowed: %s", out)
	}
	if !strings.Contains(out, `{{field "name"}}`) {
		t.Errorf("handlebars expression altered: %s", out)
	}
	if !strings.Contains(out, "{{handlebars}} expressions.") {
		t.Errorf("body text altered: %s", out)
	}
}

func TestFormat_Markdown_NoFrontmatter(t *testing.T) {
	in := "## Heading\n\nBody.\n\n\n\nExtra.\n"
	out, err := NewManager(nil).Format("markdown", in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("blank-line collapse not applied: %q", out)
	}
}

func TestFormat_Markdown_MissingClose(t *testing.T) {
	in := "---\nkey: value\n"
	out, err := NewManager(nil).Format("markdown", in)
	if !errors.Is(err, ErrMalformed) {
		t.Errorf("expected ErrMalformed, got %v", err)
	}
	if out == "" {
		t.Errorf("expected fallback content")
	}
}

func TestFormat_Lua_Tidy(t *testing.T) {
	in := "function f()   \n  return 1 \nend\n\n\n\n"
	out, err := NewManager(nil).Format("lua", in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "   \n") {
		t.Errorf("trailing whitespace not trimmed: %q", out)
	}
	if strings.HasSuffix(out, "\n\n") {
		t.Errorf("trailing newlines not collapsed: %q", out)
	}
}

func TestFormat_UnknownLang_FallsBackToTidy(t *testing.T) {
	in := "hello   \n\n\n\nworld\n"
	out, err := NewManager(nil).Format("nope", in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "   \n") || strings.Contains(out, "\n\n\n") {
		t.Errorf("tidy not applied: %q", out)
	}
}
