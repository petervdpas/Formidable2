package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestRenderMarkdown_NoTemplate(t *testing.T) {
	tpl := &template.Template{}
	got, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "No template defined") {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_Simple(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `# {{title}}`,
		Fields: []template.Field{
			{Key: "title", Type: "text"},
		},
	}
	got, err := RenderMarkdown(map[string]any{"title": "Hello"}, tpl, &Options{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "# Hello" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_FieldHelper(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `Color: {{field "color"}}`,
		Fields: []template.Field{
			{Key: "color", Type: "dropdown", Options: []any{
				map[string]any{"value": "r", "label": "Red"},
			}},
		},
	}
	got, _ := RenderMarkdown(map[string]any{"color": "r"}, tpl, &Options{})
	if got != "Color: Red" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_Loop(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `{{#loop "items"}}- {{name}}{{/loop}}`,
		Fields: []template.Field{
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		},
	}
	got, _ := RenderMarkdown(map[string]any{
		"items": []any{
			map[string]any{"name": "a"},
			map[string]any{"name": "b"},
		},
	}, tpl, &Options{})
	if got != "- a\n- b" {
		t.Errorf("got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// API-field helpers — end-to-end through RenderMarkdown.
// Confirms the full pipeline: helper registration, context plumbing,
// Options.LoadTemplate threading, output formatting.
// ─────────────────────────────────────────────────────────────────────

// apiTestSetup builds the (host template, source template, Options)
// triplet used by the api-field e2e tests below. Source has three
// fields exercising scalar / tags / table render branches.
func apiTestSetup() (*template.Template, *Options) {
	source := &template.Template{
		Filename: "addresses.yaml",
		Name:     "Street Addresses",
		Fields: []template.Field{
			{Key: "name", Type: "text", Label: "Name"},
			{Key: "tagz", Type: "tags", Label: "Tagz"},
			{Key: "owners", Type: "table", Label: "Owners",
				Options: []any{
					map[string]any{"value": "firstname", "label": "Firstname"},
					map[string]any{"value": "lastname", "label": "Lastname"},
				},
			},
		},
	}
	host := &template.Template{
		Filename: "host.yaml",
		Fields: []template.Field{
			{Key: "ref", Type: "api", Label: "Ref",
				Collection: "addresses.yaml",
				Map: []template.APIMap{
					{Key: "name", Label: "Name"},
					{Key: "tagz", Label: "Tagz"},
					{Key: "owners", Label: "OwnersAlias"},
				},
			},
		},
	}
	opts := &Options{
		LoadTemplate: func(name string) *template.Template {
			if name == "addresses.yaml" {
				return source
			}
			return nil
		},
	}
	return host, opts
}

func apiTestRow() map[string]any {
	return map[string]any{
		"ref": map[string]any{
			"guid":   "g-1",
			"name":   "Buckingham Palace",
			"tagz":   []any{"a", "b", "c"},
			"owners": []any{[]any{"Charles", "Windsor"}},
		},
	}
}

func TestRenderMarkdown_APICol_Scalar(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{apiCol "ref" "name"}}`
	got, err := RenderMarkdown(apiTestRow(), host, opts)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "Buckingham Palace" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_APICol_NonScalarFallsBackToJSON(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{apiCol "ref" "tagz"}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	if got != `["a","b","c"]` {
		t.Errorf("got %q, want JSON", got)
	}
}

func TestRenderMarkdown_APIGuid(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{apiGuid "ref"}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	if got != "g-1" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_APIBlock_Tags(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{apiBlock "ref" "tagz"}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	if got != "a, b, c" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdown_APIBlock_TableHasHeaders(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{apiBlock "ref" "owners"}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	if !strings.Contains(got, "| Firstname | Lastname |") {
		t.Errorf("missing markdown header; got:\n%s", got)
	}
	if !strings.Contains(got, "| Charles | Windsor |") {
		t.Errorf("missing markdown row; got:\n%s", got)
	}
}

func TestRenderMarkdown_APISection_FullCard(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{apiSection "ref"}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	// Card wrapper for goldmark to lift into <section class="api-card">
	if !strings.Contains(got, `<section class="api-card" data-source="addresses.yaml">`) {
		t.Errorf("missing card wrapper; got:\n%s", got)
	}
	if !strings.Contains(got, "</section>") {
		t.Errorf("missing card closer; got:\n%s", got)
	}
	// Header
	if !strings.Contains(got, "**Ref** _(addresses.yaml)_") {
		t.Errorf("missing header; got:\n%s", got)
	}
	// Inline scalar row
	if !strings.Contains(got, "- **Name**: Buckingham Palace") {
		t.Errorf("missing scalar inline row; got:\n%s", got)
	}
	// Tags block (block-shaped header + content on its own lines)
	if !strings.Contains(got, "- **Tagz**:") {
		t.Errorf("missing tagz column header; got:\n%s", got)
	}
	// Table block
	if !strings.Contains(got, "| Firstname | Lastname |") {
		t.Errorf("missing table header; got:\n%s", got)
	}
}

func TestRenderMarkdown_APIHelpers_NoRecord(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `[{{apiCol "ref" "name"}}][{{apiGuid "ref"}}][{{apiSection "ref"}}]`
	got, _ := RenderMarkdown(map[string]any{"ref": nil}, host, opts)
	if got != "[][][]" {
		t.Errorf("nil ref: want all-empty fallbacks, got %q", got)
	}
}

func TestRenderMarkdown_APIBlock_NilLoaderFallsBackToJSON(t *testing.T) {
	host, _ := apiTestSetup()
	host.MarkdownTemplate = `{{apiBlock "ref" "owners"}}`
	// Pass an Options with no LoadTemplate — apiBlock can't resolve
	// the source field's type and falls back to scalarOrJSON.
	got, _ := RenderMarkdown(apiTestRow(), host, &Options{})
	if !strings.HasPrefix(got, `[[`) {
		t.Errorf("expected JSON fallback; got %q", got)
	}
}

func TestRenderMarkdown_ParseError(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `{{#unclosed`,
	}
	_, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestRenderMarkdown_ImageStrategy(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `![logo]({{field "logo"}})`,
		Fields: []template.Field{
			{Key: "logo", Type: "image"},
		},
	}
	opts := &Options{
		ImageURL: func(name string) string { return "/storage/x/images/" + name },
	}
	got, _ := RenderMarkdown(map[string]any{"logo": "logo.png"}, tpl, opts)
	if got != "![logo](/storage/x/images/logo.png)" {
		t.Errorf("got %q", got)
	}
}
