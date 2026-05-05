package template

import (
	"testing"
)

// ─────────────────────────────────────────────────────────────────────
// Normalize — textarea format
//
// Mirrors `schemas/field.schema.js`:
//
//   const textareaFormats = new Set(["markdown", "plain"]);
//   if (field.type === "textarea") {
//     const f = String(field.format || "").toLowerCase();
//     field.format = textareaFormats.has(f) ? f : "markdown";
//   } else {
//     delete field.format;
//   }
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_TextareaMissingFormatDefaultsToMarkdown(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("missing format: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_TextareaEmptyFormatDefaultsToMarkdown(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: ""}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("empty format: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_TextareaUnknownFormatFallsBackToMarkdown(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: "html"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("unknown format: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_TextareaPreservesMarkdown(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: "markdown"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("markdown: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_TextareaPreservesPlain(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: "plain"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "plain" {
		t.Errorf("plain: want %q, got %q", "plain", got)
	}
}

func TestNormalize_TextareaLowercasesFormat(t *testing.T) {
	tpl := &Template{Fields: []Field{
		{Key: "a", Type: "textarea", Format: "MARKDOWN"},
		{Key: "b", Type: "textarea", Format: "Plain"},
	}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("MARKDOWN → want %q, got %q", "markdown", got)
	}
	if got := tpl.Fields[1].Format; got != "plain" {
		t.Errorf("Plain → want %q, got %q", "plain", got)
	}
}

func TestNormalize_TextareaTrimsWhitespace(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: "  markdown  "}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("padded format: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_NonTextareaStripsFormat(t *testing.T) {
	tpl := &Template{Fields: []Field{
		{Key: "t", Type: "text", Format: "markdown"},
		{Key: "n", Type: "number", Format: "plain"},
		{Key: "b", Type: "boolean", Format: "anything"},
	}}
	Normalize(tpl)
	for _, f := range tpl.Fields {
		if f.Format != "" {
			t.Errorf("non-textarea %s should have empty Format, got %q", f.Type, f.Format)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// Normalize — robustness (unhappy paths)
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_NilTemplateIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Normalize(nil) panicked: %v", r)
		}
	}()
	Normalize(nil)
}

func TestNormalize_NilFieldsIsSafe(t *testing.T) {
	tpl := &Template{Name: "X"}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Normalize on nil Fields panicked: %v", r)
		}
	}()
	Normalize(tpl)
}

func TestNormalize_EmptyFieldsSliceIsSafe(t *testing.T) {
	tpl := &Template{Fields: []Field{}}
	Normalize(tpl)
	if len(tpl.Fields) != 0 {
		t.Errorf("empty fields slice should stay empty")
	}
}

// ─────────────────────────────────────────────────────────────────────
// SaveTemplate integration — normalize runs on save
// ─────────────────────────────────────────────────────────────────────

func TestSaveTemplate_NormalizesTextareaFormat(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}

	// Save a textarea field with no format and a non-textarea field
	// carrying a stale format value.
	tpl := &Template{
		Name: "T",
		Fields: []Field{
			{Key: "notes", Type: "textarea"},
			{Key: "title", Type: "text", Format: "markdown"},
		},
	}
	if err := m.SaveTemplate("t.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	loaded, err := m.LoadTemplate("t.yaml")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if got := loaded.Fields[0].Format; got != "markdown" {
		t.Errorf("textarea on disk: want %q format, got %q", "markdown", got)
	}
	if got := loaded.Fields[1].Format; got != "" {
		t.Errorf("text on disk: format should be stripped, got %q", got)
	}
}
