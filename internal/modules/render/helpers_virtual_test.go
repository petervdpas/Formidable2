package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// {{virtual-field "fieldKey"}} renders a virtual (data-less) field's
// value as a string. Today the only virtual type is `facet`; the
// helper looks the field up by key on the template, dispatches on
// type, and returns the projection. Argument is the field key on the
// template - NOT the facet key - so the same call shape will work
// for any future virtual type (computed, derived, etc.).

func TestVirtualFieldHelper_FacetReturnsSelectedLabel(t *testing.T) {
	tpl := &template.Template{
		Name:             "tpl",
		Filename:         "tpl.yaml",
		MarkdownTemplate: `Status: {{virtual-field "status_inline"}}`,
		Facets: []template.Facet{{
			Key:     "status",
			Icon:    "fa-flag",
			Options: []template.FacetOption{{Label: "OPEN", Color: "blue"}},
		}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio"},
		},
	}
	opts := &Options{Facets: map[string]string{"status": "OPEN"}}
	got, err := RenderMarkdown(map[string]any{"title": "Hello"}, tpl, opts)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(got, "Status: OPEN") {
		t.Errorf("expected 'Status: OPEN' in output; got %q", got)
	}
}

func TestVirtualFieldHelper_FacetUnsetReturnsEmpty(t *testing.T) {
	tpl := &template.Template{
		Name:             "tpl",
		Filename:         "tpl.yaml",
		MarkdownTemplate: `Status: [{{virtual-field "status_inline"}}]`,
		Facets: []template.Facet{{
			Key:     "status",
			Icon:    "fa-flag",
			Options: []template.FacetOption{{Label: "OPEN", Color: "blue"}},
		}},
		Fields: []template.Field{
			{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio"},
		},
	}
	got, err := RenderMarkdown(nil, tpl, &Options{})
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(got, "Status: []") {
		t.Errorf("expected 'Status: []' for unset facet; got %q", got)
	}
}

func TestVirtualFieldHelper_UnknownKeyReturnsEmpty(t *testing.T) {
	tpl := &template.Template{
		Name:             "tpl",
		Filename:         "tpl.yaml",
		MarkdownTemplate: `[{{virtual-field "ghost"}}]`,
		Fields: []template.Field{
			{Key: "title", Type: "text"},
		},
	}
	got, err := RenderMarkdown(nil, tpl, &Options{})
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(got, "[]") {
		t.Errorf("unknown field key must render empty; got %q", got)
	}
}

func TestVirtualFieldHelper_NonVirtualTypeReturnsEmpty(t *testing.T) {
	// Calling {{virtual-field "title"}} on a regular text field is
	// nonsensical but should fail-safe to empty rather than the field's
	// data value (use {{field "title"}} for that).
	tpl := &template.Template{
		Name:             "tpl",
		Filename:         "tpl.yaml",
		MarkdownTemplate: `[{{virtual-field "title"}}]`,
		Fields: []template.Field{
			{Key: "title", Type: "text"},
		},
	}
	got, err := RenderMarkdown(map[string]any{"title": "Hello"}, tpl, &Options{})
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(got, "[]") {
		t.Errorf("non-virtual field type must render empty; got %q", got)
	}
}
