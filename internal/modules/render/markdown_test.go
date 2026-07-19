package render

import (
	"strings"
	"sync"
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

func TestRenderMarkdown_APITable_HeaderAndRow(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{apiTable "ref"}}`
	got, err := RenderMarkdown(apiTestRow(), host, opts)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "| Name | Tagz | OwnersAlias |") {
		t.Errorf("missing header from Map columns; got:\n%s", got)
	}
	if !strings.Contains(got, "| --- | --- | --- |") {
		t.Errorf("missing separator row; got:\n%s", got)
	}
	// One row per referenced record; scalar passes through, tags join inline.
	if !strings.Contains(got, "Buckingham Palace") || !strings.Contains(got, "a, b, c") {
		t.Errorf("missing data-row cells; got:\n%s", got)
	}
}

func TestRenderMarkdown_APITable_NamedColumnsOnly(t *testing.T) {
	host, opts := apiTestSetup()
	// Pick only "name", and reverse-ish order is honored too.
	host.MarkdownTemplate = `{{apiTable "ref" "name"}}`
	got, err := RenderMarkdown(apiTestRow(), host, opts)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "| Name |") || !strings.Contains(got, "Buckingham Palace") {
		t.Errorf("expected only the Name column; got:\n%s", got)
	}
	// Columns not named must not appear.
	if strings.Contains(got, "Tagz") || strings.Contains(got, "OwnersAlias") {
		t.Errorf("unnamed columns leaked into the table; got:\n%s", got)
	}
}

func TestRenderMarkdown_APITable_NoRecordIsEmpty(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `[{{apiTable "ref"}}]`
	got, _ := RenderMarkdown(map[string]any{"ref": nil}, host, opts)
	if got != "[]" {
		t.Errorf("unpicked ref should render empty; got %q", got)
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

// TestRenderMarkdown_MalformedBlockMismatch covers a structurally invalid
// Handlebars template (block opener does not match its closer). raymond
// surfaces a parse error wrapped by RenderMarkdown; it must not panic and
// must return empty output alongside the error.
func TestRenderMarkdown_MalformedBlockMismatch(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `{{#loop "a"}}x{{/wrong}}`}
	out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err == nil {
		t.Fatal("expected parse error for mismatched block, got nil")
	}
	if out != "" {
		t.Errorf("want empty output on parse error, got %q", out)
	}
	if !strings.Contains(err.Error(), "render: parse markdown template") {
		t.Errorf("want wrapped parse error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "doesn't match") {
		t.Errorf("want raymond mismatch detail, got %q", err.Error())
	}
}

// TestRenderMarkdown_UnclosedExpression covers an unterminated mustache.
// The parse stage fails and the error is wrapped, no panic.
func TestRenderMarkdown_UnclosedExpression(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `a {{title`}
	out, err := RenderMarkdown(map[string]any{"title": "x"}, tpl, &Options{})
	if err == nil {
		t.Fatal("expected parse error for unclosed expression")
	}
	if out != "" {
		t.Errorf("want empty output, got %q", out)
	}
	if !strings.Contains(err.Error(), "render: parse markdown template") {
		t.Errorf("want wrapped parse error, got %q", err.Error())
	}
}

// TestRenderMarkdown_UnknownHelperWithArgs pins raymond's current behavior:
// an unregistered helper invoked with positional args expands to "" with no
// error (it is not treated as a missing variable, and it does not error).
func TestRenderMarkdown_UnknownHelperWithArgs(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `[{{nosuchhelper "x"}}]`}
	out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error for unknown helper: %v", err)
	}
	if out != "[]" {
		t.Errorf("unknown helper with args: want empty expansion, got %q", out)
	}
}

// TestRenderMarkdown_UnknownHelperArgless pins the argless case: a bare
// {{bogus}} is resolved as a missing context variable, expanding to "".
func TestRenderMarkdown_UnknownHelperArgless(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `a{{bogus}}b`}
	out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ab" {
		t.Errorf("argless unknown: want %q, got %q", "ab", out)
	}
}

// TestRenderMarkdown_FieldHelper_MissingField covers {{field}} referencing a
// key with no matching Field. The helper returns the explicit
// "(unknown field: <key>)" sentinel rather than panicking on a nil Field.
func TestRenderMarkdown_FieldHelper_MissingField(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `[{{field "ghost"}}]`}
	out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[(unknown field: ghost)]" {
		t.Errorf("missing field: want sentinel, got %q", out)
	}
}

// TestRenderMarkdown_FieldDescription_MissingField covers a second helper
// that dereferences a Field: a missing key must yield "" not a panic.
func TestRenderMarkdown_FieldDescription_MissingField(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `[{{fieldDescription "ghost"}}]`}
	out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[]" {
		t.Errorf("missing field description: want %q, got %q", "[]", out)
	}
}

// TestRenderMarkdown_ImageField_NonString covers an image field whose stored
// value is not a string (e.g. a number from a corrupt data file). emitImage
// type-asserts and bails to "" rather than panicking.
func TestRenderMarkdown_ImageField_NonString(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `[{{field "logo"}}]`,
		Fields:           []template.Field{{Key: "logo", Type: "image"}},
	}
	out, err := RenderMarkdown(map[string]any{"logo": 12345}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[]" {
		t.Errorf("non-string image: want empty, got %q", out)
	}
}

// TestRenderMarkdown_ImageField_EmptyName covers an empty image filename:
// emitImage returns "" before consulting ImageURL.
func TestRenderMarkdown_ImageField_EmptyName(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `[{{field "logo"}}]`,
		Fields:           []template.Field{{Key: "logo", Type: "image"}},
	}
	out, err := RenderMarkdown(map[string]any{"logo": ""}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[]" {
		t.Errorf("empty image name: want %q, got %q", "[]", out)
	}
}

// TestRenderMarkdown_EmptyLoop covers an empty loop slice: the block body
// runs zero times and contributes nothing between the surrounding markers.
func TestRenderMarkdown_EmptyLoop(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `[{{#loop "items"}}- {{name}}{{/loop}}]`,
		Fields: []template.Field{
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		},
	}
	out, err := RenderMarkdown(map[string]any{"items": []any{}}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[]" {
		t.Errorf("empty loop: want %q, got %q", "[]", out)
	}
}

// TestRenderMarkdown_MissingLoopKey covers a loop whose key is absent from
// the data (not just empty). The helper's []any assertion fails and it
// returns "" without iterating, matching the empty-slice case.
func TestRenderMarkdown_MissingLoopKey(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `[{{#loop "items"}}- {{name}}{{/loop}}]`,
		Fields: []template.Field{
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		},
	}
	out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[]" {
		t.Errorf("missing loop key: want %q, got %q", "[]", out)
	}
}

// TestRenderMarkdown_NestedLoopWithEmptyInner covers a deeply nested loop
// where one outer entry carries inner items and another carries an empty
// inner slice. Pins the exact join shape: outer entries joined by "\n",
// inner expansions inline, empty inner contributing nothing.
func TestRenderMarkdown_NestedLoopWithEmptyInner(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `{{#loop "outer"}}O:{{name}}{{#loop "inner"}} I:{{val}}{{/loop}}{{/loop}}`,
		Fields: []template.Field{
			{Key: "outer", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "inner", Type: "loopstart"},
			{Key: "val", Type: "text"},
			{Key: "inner", Type: "loopstop"},
			{Key: "outer", Type: "loopstop"},
		},
	}
	out, err := RenderMarkdown(map[string]any{
		"outer": []any{
			map[string]any{"name": "A", "inner": []any{
				map[string]any{"val": "1"},
				map[string]any{"val": "2"},
			}},
			map[string]any{"name": "B", "inner": []any{}},
		},
	}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "O:A I:1\n I:2\nO:B" {
		t.Errorf("nested loop shape: got %q", out)
	}
}

// TestRenderMarkdown_LoopWrap_GoldmarkType6 covers the goldmark type-6
// HTML-block path used by the loop-item wrappers. The blank lines emitted by
// loopItemBefore/After must let the inner markdown ("# Hi") parse to an
// <h1> while still sitting inside the <section> wrapper.
func TestRenderMarkdown_LoopWrap_GoldmarkType6(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: "{{#loop \"items\"}}{{loopItemBefore}}# {{name}}{{loopItemAfter}}{{/loop}}",
		Fields: []template.Field{
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		},
	}
	md, err := RenderMarkdown(map[string]any{"items": []any{map[string]any{"name": "Hi"}}}, tpl, &Options{})
	if err != nil {
		t.Fatalf("markdown err: %v", err)
	}
	// The wrapper must carry the blank-line gaps that drive type-6 parsing.
	if !strings.Contains(md, "data-loop=\"items\" data-index=\"1\">\n\n# Hi\n\n</section>") {
		t.Errorf("wrapper markdown shape wrong: %q", md)
	}
	html, err := RenderHTML(md)
	if err != nil {
		t.Fatalf("html err: %v", err)
	}
	// Inner markdown parsed despite the surrounding HTML block.
	if !strings.Contains(html, "<h1>Hi</h1>") {
		t.Errorf("inner markdown did not parse to <h1>: %q", html)
	}
	// Section wrapper survived around the parsed heading.
	if !strings.Contains(html, `<section class="loop-item"`) {
		t.Errorf("section wrapper lost: %q", html)
	}
}

// TestRenderMarkdown_APISection_GoldmarkType6 covers the same type-6 trick
// for the api-card wrapper: the blank lines around the section must let the
// inner bold markdown parse to <strong> inside <section class="api-card">.
func TestRenderMarkdown_APISection_GoldmarkType6(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `{{apiSection "ref"}}`
	md, err := RenderMarkdown(apiTestRow(), host, opts)
	if err != nil {
		t.Fatalf("markdown err: %v", err)
	}
	html, err := RenderHTML(md)
	if err != nil {
		t.Fatalf("html err: %v", err)
	}
	if !strings.Contains(html, `<section class="api-card" data-source="addresses.yaml">`) {
		t.Errorf("api-card wrapper lost: %q", html)
	}
	// Inner bold parsed: the header **Ref** became <strong>Ref</strong>.
	if !strings.Contains(html, "<strong>Ref</strong>") {
		t.Errorf("inner bold did not parse to <strong>: %q", html)
	}
	// Block-typed table column parsed to a real HTML table inside the card.
	if !strings.Contains(html, "<table>") {
		t.Errorf("inner table block did not parse: %q", html)
	}
}

// TestRenderMarkdown_APISection_NilHostField covers apiSection when the host
// field cannot be resolved (no matching api Field). It must return "" so the
// template degrades to empty rather than dereferencing a nil *Field.
func TestRenderMarkdown_APISection_NilHostField(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `[{{apiSection "ghost"}}]`}
	out, err := RenderMarkdown(map[string]any{"ghost": map[string]any{"guid": "g"}}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[]" {
		t.Errorf("apiSection nil host field: want %q, got %q", "[]", out)
	}
}

// TestRenderMarkdown_UnknownBlockHelper pins raymond's behavior for an
// unregistered block helper: {{#nosuch}}..{{/nosuch}} resolves the name as a
// falsy private context, the body runs zero times, and the result is "" with
// no error (no panic on the missing helper).
func TestRenderMarkdown_UnknownBlockHelper(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `[{{#nosuch}}body{{/nosuch}}]`}
	out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error for unknown block helper: %v", err)
	}
	if out != "[]" {
		t.Errorf("unknown block helper: want %q, got %q", "[]", out)
	}
}

// TestRenderMarkdown_ImageField_URLResolve drives emitImage's resolve path:
// a relative image filename with Options.ImageURL set must route through the
// resolver, not the "images/<file>" fallback.
func TestRenderMarkdown_ImageField_URLResolve(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `[{{field "logo"}}]`,
		Fields:           []template.Field{{Key: "logo", Type: "image"}},
	}
	opts := &Options{ImageURL: func(name string) string { return "https://cdn/" + name }}
	out, err := RenderMarkdown(map[string]any{"logo": "x.png"}, tpl, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[https://cdn/x.png]" {
		t.Errorf("image url resolve: want %q, got %q", "[https://cdn/x.png]", out)
	}
}

// TestRenderMarkdown_ImageField_AbsolutePassthrough covers emitImage's
// absolute-URL short circuit: an http(s) value bypasses ImageURL entirely.
func TestRenderMarkdown_ImageField_AbsolutePassthrough(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `[{{field "logo"}}]`,
		Fields:           []template.Field{{Key: "logo", Type: "image"}},
	}
	opts := &Options{ImageURL: func(name string) string { return "SHOULD-NOT-RUN/" + name }}
	out, err := RenderMarkdown(map[string]any{"logo": "https://abs/y.png"}, tpl, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[https://abs/y.png]" {
		t.Errorf("absolute image passthrough: want %q, got %q", "[https://abs/y.png]", out)
	}
}

// TestRenderMarkdown_ImageField_NoResolver covers the nil-ImageURL fallback:
// a relative filename resolves to the static "images/<file>" path.
func TestRenderMarkdown_ImageField_NoResolver(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `[{{field "logo"}}]`,
		Fields:           []template.Field{{Key: "logo", Type: "image"}},
	}
	out, err := RenderMarkdown(map[string]any{"logo": "x.png"}, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[images/x.png]" {
		t.Errorf("image fallback: want %q, got %q", "[images/x.png]", out)
	}
}

// TestRenderMarkdown_LinkField_MalformedFormidable covers a malformed
// formidable:// href (no ":" datafile separator). parseFormidableURL fails,
// so resolveLinkHref passes the original URL through untouched rather than
// dropping or half-rewriting the link.
func TestRenderMarkdown_LinkField_MalformedFormidable(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `{{field "ln"}}`,
		Fields:           []template.Field{{Key: "ln", Type: "link"}},
	}
	called := false
	opts := &Options{FormidableLinkURL: func(_, _ string) string {
		called = true
		return "REWRITTEN"
	}}
	out, err := RenderMarkdown(map[string]any{"ln": "formidable://broken"}, tpl, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("rewriter must not run for a malformed formidable:// URL")
	}
	if out != "[formidable://broken](formidable://broken)" {
		t.Errorf("malformed formidable link: got %q", out)
	}
}

// TestRenderMarkdown_APIBlock_NilSourceTemplate covers apiBlock when the value
// is fetched live but the source template cannot be loaded (LoadTemplate returns
// nil for a since-removed collection). emitAPIColumnBlock falls back to
// scalarOrJSON, here compact JSON for the slice value, rather than crashing.
func TestRenderMarkdown_APIBlock_NilSourceTemplate(t *testing.T) {
	host := &template.Template{
		MarkdownTemplate: `{{apiBlock "ref" "gone"}}`,
		Fields: []template.Field{{Key: "ref", Type: "api", Collection: "src.yaml",
			Map: []template.APIMap{{Key: "gone"}}}},
	}
	opts := &Options{
		LoadTemplate: func(string) *template.Template { return nil },
		ResolveReference: func(_, _ string, _ []string) map[string]any {
			return map[string]any{"gone": []any{"a", "b"}}
		},
	}
	row := map[string]any{"ref": "g"}
	out, err := RenderMarkdown(row, host, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != `["a","b"]` {
		t.Errorf("apiBlock nil source: want JSON fallback, got %q", out)
	}
}

// TestRenderMarkdown_APICol_UnpickedRecord covers apiCol when the api field
// has no picked record (key absent from the row). apiFieldRow returns nil so
// the helper emits "" rather than dereferencing a nil map.
func TestRenderMarkdown_APICol_UnpickedRecord(t *testing.T) {
	host, opts := apiTestSetup()
	host.MarkdownTemplate = `[{{apiCol "ref" "name"}}]`
	out, err := RenderMarkdown(map[string]any{}, host, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[]" {
		t.Errorf("apiCol unpicked: want %q, got %q", "[]", out)
	}
}

// TestRenderMarkdown_ConcurrentSetVarScratch hammers RenderMarkdown from many
// goroutines. setVar/getVar use a per-call scratch map, so a shared-state race
// would either fail -race or cross-contaminate values. Each call must read
// back exactly its own write.
func TestRenderMarkdown_ConcurrentSetVarScratch(t *testing.T) {
	tpl := &template.Template{MarkdownTemplate: `{{setVar "x" "v"}}[{{getVar "x"}}]`}
	const n = 32
	var wg sync.WaitGroup
	errs := make(chan string, n)
	for range n {
		wg.Go(func() {
			out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
			if err != nil {
				errs <- err.Error()
				return
			}
			if out != "[v]" {
				errs <- out
			}
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Errorf("concurrent render mismatch: %q", e)
	}
}

// TestRenderMarkdown_NilTemplate covers the nil *Template guard: RenderMarkdown
// returns the "No template defined." sentinel without error or panic.
func TestRenderMarkdown_NilTemplate(t *testing.T) {
	out, err := RenderMarkdown(map[string]any{}, nil, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "# No template defined." {
		t.Errorf("nil template: want sentinel, got %q", out)
	}
}

// TestRenderMarkdown_SlideHelper: the {{slide}} helper renders the record's
// slide field without the author naming its key, mirroring {{board}}.
func TestRenderMarkdown_SlideHelper(t *testing.T) {
	tpl := &template.Template{
		Presentation:     true,
		MarkdownTemplate: "{{slide}}",
		Fields: []template.Field{
			{Key: "slide", Type: "slide"},
		},
	}
	values := map[string]any{"slide": map[string]any{"blocks": []any{
		map[string]any{"id": "b1", "kind": "text", "content": "## Hello World",
			"x": float64(40), "y": float64(60), "w": float64(600), "h": float64(200)},
	}}}
	out, err := RenderMarkdown(values, tpl, &Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{`class="slide-canvas"`, "Hello World"} {
		if !strings.Contains(out, want) {
			t.Errorf("{{slide}} render missing %q\n---\n%s", want, out)
		}
	}
}
