package render

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

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

// TestRenderMarkdown_TemplateLoadError covers Manager.RenderMarkdown when the
// template loader fails: the error is wrapped with the template name and the
// %w sentinel from the loader is preserved for errors.Is.
func TestRenderMarkdown_TemplateLoadError(t *testing.T) {
	sentinel := errors.New("boom")
	m := NewManager(
		&fakeTemplateLoader{err: sentinel},
		&fakeFormStore{},
		nil, nil, nil,
	)
	out, err := m.RenderMarkdown("missing.yaml", "")
	if err == nil {
		t.Fatal("expected load error")
	}
	if out != "" {
		t.Errorf("want empty output, got %q", out)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("want wrapped sentinel via %%w, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), `render: load template "missing.yaml"`) {
		t.Errorf("want template name in error, got %q", err.Error())
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
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := RenderMarkdown(map[string]any{}, tpl, &Options{})
			if err != nil {
				errs <- err.Error()
				return
			}
			if out != "[v]" {
				errs <- out
			}
		}()
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
