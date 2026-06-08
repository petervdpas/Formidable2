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
// API-field helpers - end-to-end through RenderMarkdown.
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
	// The live record the resolver returns for g-1; columns are read live now,
	// not snapshotted into the host form.
	record := map[string]any{
		"name":   "Buckingham Palace",
		"tagz":   []any{"a", "b", "c"},
		"owners": []any{[]any{"Charles", "Windsor"}},
	}
	opts := &Options{
		LoadTemplate: func(name string) *template.Template {
			if name == "addresses.yaml" {
				return source
			}
			return nil
		},
		ResolveReference: func(tpl, id string, cols []string) map[string]any {
			if tpl != "addresses.yaml" || id != "g-1" {
				return nil
			}
			row := map[string]any{}
			for _, c := range cols {
				row[c] = record[c]
			}
			return row
		},
	}
	return host, opts
}

func apiTestRow() map[string]any {
	// The field stores only the reference id; the columns come from the resolver.
	return map[string]any{"ref": "g-1"}
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
	host, opts := apiTestSetup()
	// Resolver present (values fetched), but no LoadTemplate - apiBlock can't
	// resolve the source field's type and falls back to scalarOrJSON.
	opts.LoadTemplate = nil
	host.MarkdownTemplate = `{{apiBlock "ref" "owners"}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	if !strings.HasPrefix(got, `[[`) {
		t.Errorf("expected JSON fallback; got %q", got)
	}
}

func TestRenderMarkdown_APIBlock_NoResolverRendersEmpty(t *testing.T) {
	host, _ := apiTestSetup()
	host.MarkdownTemplate = `[{{apiBlock "ref" "owners"}}]`
	// No resolver: the id is known but no live data can be fetched -> empty.
	got, _ := RenderMarkdown(apiTestRow(), host, &Options{})
	if got != "[]" {
		t.Errorf("no resolver should render empty; got %q", got)
	}
}

func TestRenderMarkdown_APISection_ToManyRendersCardPerRecord(t *testing.T) {
	source := &template.Template{
		Filename: "addresses.yaml",
		Fields:   []template.Field{{Key: "name", Type: "text", Label: "Name"}},
	}
	host := &template.Template{
		Filename: "host.yaml",
		Fields: []template.Field{
			{Key: "refs", Type: "api", Label: "Refs", Collection: "addresses.yaml",
				Map: []template.APIMap{{Key: "name", Label: "Name"}}},
		},
		MarkdownTemplate: `{{apiSection "refs"}}`,
	}
	names := map[string]string{"g-1": "Alpha", "g-2": "Beta"}
	opts := &Options{
		LoadTemplate: func(string) *template.Template { return source },
		ResolveReference: func(_, id string, _ []string) map[string]any {
			if n, ok := names[id]; ok {
				return map[string]any{"name": n}
			}
			return nil
		},
	}
	got, _ := RenderMarkdown(map[string]any{"refs": []any{"g-1", "g-2"}}, host, opts)
	if !strings.Contains(got, "Alpha") || !strings.Contains(got, "Beta") {
		t.Errorf("to-many should render a card per record; got:\n%s", got)
	}
	if strings.Count(got, `<section class="api-card"`) != 2 {
		t.Errorf("expected 2 cards; got:\n%s", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// api-field iteration: {{#each (apiRows "key")}} exposes the resolved
// records (columns + _link + _labels) for author-built markup.
// ─────────────────────────────────────────────────────────────────────

func TestRenderMarkdown_APISection_FirstColumnLinksToRecord(t *testing.T) {
	host, opts := apiTestSetup()
	opts.ResolveReferenceLink = func(target, id string) string {
		if target == "addresses.yaml" && id == "g-1" {
			return "formidable://addresses.yaml:buckingham.meta.json"
		}
		return ""
	}
	host.MarkdownTemplate = `{{apiSection "ref"}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	// First mapped column's VALUE links to the record (slideout keeps
	// formidable:// because no FormidableLinkURL rewriter is wired here).
	if !strings.Contains(got, "- **Name**: [Buckingham Palace](formidable://addresses.yaml:buckingham.meta.json)") {
		t.Errorf("first column value should link to the record; got:\n%s", got)
	}
	// The header stays plain.
	if strings.Contains(got, "[Ref](") {
		t.Errorf("header should not be linked; got:\n%s", got)
	}
}

func TestRenderMarkdown_APISection_CardHeaderPlainWithoutResolver(t *testing.T) {
	host, opts := apiTestSetup() // no ResolveReferenceLink wired
	host.MarkdownTemplate = `{{apiSection "ref"}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	if strings.Contains(got, "](formidable://") {
		t.Errorf("no resolver should leave the value plain; got:\n%s", got)
	}
	if !strings.Contains(got, "- **Name**: Buckingham Palace") {
		t.Errorf("expected plain value; got:\n%s", got)
	}
}

func TestRenderMarkdown_APIRows_NamedValueLinkAndLabels(t *testing.T) {
	host, opts := apiTestSetup()
	opts.ResolveReferenceLink = func(target, id string) string {
		if target == "addresses.yaml" && id == "g-1" {
			return "formidable://addresses.yaml:buckingham.meta.json"
		}
		return ""
	}
	// Named value, the record link, and the column label - all per row.
	host.MarkdownTemplate = `{{#each (apiRows "ref")}}{{this.labels.name}}: [{{this.name}}]({{this.link}}){{/each}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	want := "Name: [Buckingham Palace](formidable://addresses.yaml:buckingham.meta.json)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderMarkdown_APIRows_TableWithFieldMetaHeader(t *testing.T) {
	source := &template.Template{
		Filename: "nomenclature.yaml",
		Fields: []template.Field{
			{Key: "term", Type: "text", Label: "Term"},
			{Key: "description", Type: "textarea", Label: "Omschrijving"},
		},
	}
	host := &template.Template{
		Filename: "host.yaml",
		Fields: []template.Field{
			{Key: "refs", Type: "api", Label: "Refs", Collection: "nomenclature.yaml",
				Map: []template.APIMap{{Key: "term", Label: "Term"}, {Key: "description", Label: "Omschrijving"}}},
		},
		// Header from fieldMeta, rows from apiRows with named column access.
		MarkdownTemplate: "{{#with (fieldMeta \"refs\" \"options\") as |cols|}}|{{#each cols}} {{label}} |{{/each}}\n|{{#each cols}} --- |{{/each}}\n{{/with}}{{#each (apiRows \"refs\")}}| {{this.term}} | {{this.description}} |\n{{/each}}",
	}
	recs := map[string]map[string]any{
		"g-1": {"term": "Push", "description": "Het versturen"},
		"g-2": {"term": "Pull", "description": "Het ophalen"},
	}
	opts := &Options{
		LoadTemplate:     func(string) *template.Template { return source },
		ResolveReference: func(_, id string, _ []string) map[string]any { return recs[id] },
	}
	got, _ := RenderMarkdown(map[string]any{"refs": []any{"g-1", "g-2"}}, host, opts)
	for _, want := range []string{"| Term | Omschrijving |", "| Push | Het versturen |", "| Pull | Het ophalen |"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q; got:\n%s", want, got)
		}
	}
}

func TestRenderMarkdown_APIRows_EmptyWhenUnpicked(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `[{{#each (apiRows "ref")}}{{this.name}}{{/each}}]`
	got, _ := RenderMarkdown(map[string]any{"ref": nil}, host, opts)
	if got != "[]" {
		t.Errorf("unpicked apiRows should iterate nothing; got %q", got)
	}
}

func TestRenderMarkdown_APIRows_BareColumnAccess(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{#each (apiRows "ref")}}{{name}}{{/each}}`
	got, _ := RenderMarkdown(apiTestRow(), host, opts)
	if got != "Buckingham Palace" {
		t.Errorf("got %q", got)
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
