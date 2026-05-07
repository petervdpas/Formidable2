package render

import (
	"strings"
	"testing"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// renderWithCtx is a tiny helper that mirrors the per-call setup
// RenderMarkdown does: parse a snippet, register helpers, exec.
func renderWithCtx(t *testing.T, src string, ctx map[string]any) string {
	t.Helper()
	tpl, err := raymond.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	registerHelpers(tpl, &Options{}, map[string]any{})
	out, err := tpl.Exec(ctx)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	return out
}

func ctxFromTemplate(tpl *template.Template, values map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range values {
		out[k] = v
	}
	out["_fields"] = tpl.Fields
	out["_template"] = tpl
	out["_loopGroups"] = buildNestedLoopGroups(tpl.Fields)
	return out
}

func TestHelper_Eq(t *testing.T) {
	got := renderWithCtx(t, `{{#if (eq a b)}}match{{else}}no{{/if}}`, map[string]any{
		"a": 1, "b": 1,
	})
	if got != "match" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_MathAdd(t *testing.T) {
	got := renderWithCtx(t, `{{add a b}}`, map[string]any{"a": 2, "b": 3})
	if got != "5" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Length(t *testing.T) {
	got := renderWithCtx(t, `{{length items}}`, map[string]any{
		"items": []any{"x", "y", "z"},
	})
	if got != "3" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Includes(t *testing.T) {
	got := renderWithCtx(t, `{{#if (includes items "x")}}yes{{/if}}`, map[string]any{
		"items": []any{"a", "x"},
	})
	if got != "yes" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_PascalCamel(t *testing.T) {
	got := renderWithCtx(t, `{{pascal "hello"}} {{camel "Hello"}}`, map[string]any{})
	if got != "Hello hello" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_LookupOption(t *testing.T) {
	opts := []any{
		map[string]any{"value": "r", "label": "Red"},
		map[string]any{"value": "g", "label": "Green"},
	}
	got := renderWithCtx(t, `{{#with (lookupOption options "g")}}{{label}}{{/with}}`, map[string]any{
		"options": opts,
	})
	if got != "Green" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_FieldText(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{Key: "name", Type: "text"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"name": "Alice"})
	got := renderWithCtx(t, `{{field "name"}}`, ctx)
	if got != "Alice" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_FieldUnknownKey(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{}}
	ctx := ctxFromTemplate(tpl, map[string]any{})
	got := renderWithCtx(t, `{{field "missing"}}`, ctx)
	if !strings.Contains(got, "unknown field") {
		t.Errorf("got %q", got)
	}
}

func TestHelper_FieldDropdownLabel(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{
			Key: "color", Type: "dropdown",
			Options: []any{
				map[string]any{"value": "r", "label": "Red"},
				map[string]any{"value": "b", "label": "Blue"},
			},
		}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"color": "r"})
	got := renderWithCtx(t, `{{field "color"}}`, ctx)
	if got != "Red" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_FieldDropdownValueMode(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{
			Key: "color", Type: "dropdown",
			Options: []any{
				map[string]any{"value": "r", "label": "Red"},
			},
		}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"color": "r"})
	got := renderWithCtx(t, `{{field "color" mode="value"}}`, ctx)
	if got != "r" {
		t.Errorf("got %q", got)
	}
}

// Original Formidable's basic.yaml uses `{{field "key" "value"}}` —
// 2 positional args where the second is the mode. Raymond's strict
// arity rejected that until the options-only helper patch landed.
// See third_party/raymond/CHANGES.md "options-only variadic helpers".
// `{{field "linkkey"}}` (no mode) on a link field must emit a
// Markdown link, not the label alone — matches the original JS's
// behaviour where the function's mode-default got shadowed by
// handlebars.js's arity quirk and fell through to the markdown-link
// branch. Without this default, recipe-style `Gerelateerde recepten`
// blocks render as plain text and the wiki/ slideout interceptors
// have no <a> to hook into.
func TestHelper_FieldLinkDefaultIsMarkdownLink(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{Key: "ref", Type: "link"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{
		"ref": map[string]any{
			"href": "formidable://other.yaml:other.meta.json",
			"text": "See other",
		},
	})
	got := renderWithCtx(t, `{{field "ref"}}`, ctx)
	if got != "[See other](formidable://other.yaml:other.meta.json)" {
		t.Errorf("link default should be markdown link; got %q", got)
	}
}

// Explicit `mode="label"` should still produce label-only output.
// The default-only fallthrough must not strip the user's ability to
// ask for the label.
func TestHelper_FieldLinkExplicitLabelStillTextOnly(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{Key: "ref", Type: "link"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{
		"ref": map[string]any{
			"href": "formidable://other.yaml:y.meta.json",
			"text": "Hello",
		},
	})
	got := renderWithCtx(t, `{{field "ref" "label"}}`, ctx)
	if got != "Hello" {
		t.Errorf("explicit mode=label should return text only; got %q", got)
	}
}

func TestHelper_FieldDropdownPositionalMode(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{
			Key: "color", Type: "dropdown",
			Options: []any{
				map[string]any{"value": "r", "label": "Red"},
			},
		}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"color": "r"})
	got := renderWithCtx(t, `{{field "color" "value"}}`, ctx)
	if got != "r" {
		t.Errorf("positional mode: got %q, want r", got)
	}
}

func TestHelper_FieldHashWinsOverPositional(t *testing.T) {
	// When both forms are provided, the hash mode= takes precedence —
	// matches the JS helper's hash precedence and is what the editor
	// relies on for "force a mode override".
	tpl := &template.Template{
		Fields: []template.Field{{
			Key: "color", Type: "dropdown",
			Options: []any{
				map[string]any{"value": "r", "label": "Red"},
			},
		}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"color": "r"})
	got := renderWithCtx(t, `{{field "color" "value" mode="label"}}`, ctx)
	if got != "Red" {
		t.Errorf("hash should win: got %q, want Red", got)
	}
}

func TestHelper_FieldRaw(t *testing.T) {
	ctx := map[string]any{"x": 42}
	got := renderWithCtx(t, `{{fieldRaw "x"}}`, ctx)
	if got != "42" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_FieldDescription(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{Key: "x", Type: "text", Description: "hello"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{})
	got := renderWithCtx(t, `{{fieldDescription "x"}}`, ctx)
	if got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Loop(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{
		"items": []any{
			map[string]any{"name": "a"},
			map[string]any{"name": "b"},
		},
	})
	got := renderWithCtx(t, `{{#loop "items"}}- {{field "name"}} ({{items_index}}){{/loop}}`, ctx)
	want := "- a (1)\n- b (2)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHelper_Cell(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{
			Key: "grid", Type: "table",
			Options: []any{"col1", "col2", "col3"},
		}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{
		"row": []any{"a", "b", "c"},
	})
	got := renderWithCtx(t, `{{cell row "col2" "grid"}}`, ctx)
	if got != "b" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_SetGetVar(t *testing.T) {
	got := renderWithCtx(t, `{{setVar "x" 42}}{{getVar "x"}}`, map[string]any{})
	if got != "42" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_SetVarPerCall(t *testing.T) {
	// Two separate render calls must NOT leak state — original JS
	// version had a module-level `vars` bug; we use per-call scratch.
	tpl1, _ := raymond.Parse(`{{setVar "x" "first"}}`)
	tpl2, _ := raymond.Parse(`{{getVar "x"}}`)
	registerHelpers(tpl1, &Options{}, map[string]any{})
	registerHelpers(tpl2, &Options{}, map[string]any{})
	_, _ = tpl1.Exec(map[string]any{})
	out, _ := tpl2.Exec(map[string]any{})
	if out != "" {
		t.Errorf("vars leaked across renders: %q", out)
	}
}

func TestHelper_JSON(t *testing.T) {
	got := renderWithCtx(t, `{{json items}}`, map[string]any{
		"items": []any{"a", "b"},
	})
	if !strings.Contains(got, `"a"`) || !strings.Contains(got, `"b"`) {
		t.Errorf("got %q", got)
	}
}

func TestHelper_TagsHash(t *testing.T) {
	got := renderWithCtx(t, `{{tags items}}`, map[string]any{
		"items": []any{"Foo Bar"},
	})
	if got != "#foo-bar" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_TagsNoHash(t *testing.T) {
	got := renderWithCtx(t, `{{tags items withHash=false}}`, map[string]any{
		"items": []any{"Foo Bar"},
	})
	if got != "foo-bar" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_TagsZeroArg(t *testing.T) {
	// {{tags}} with no positional arg — JS defaults array to []. Go now
	// matches via the options-only signature; should return empty string,
	// not error.
	got := renderWithCtx(t, `[{{tags}}]`, map[string]any{})
	if got != "[]" {
		t.Errorf("got %q, want []", got)
	}
}

func TestHelper_IsSelected(t *testing.T) {
	got := renderWithCtx(t, `{{#isSelected items "x"}}yes{{else}}no{{/isSelected}}`, map[string]any{
		"items": []any{"a", "x"},
	})
	if got != "yes" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_MathOpExposed(t *testing.T) {
	// Generic `math` helper takes (a, op, b).
	got := renderWithCtx(t, `{{math 6 "/" 2}}`, map[string]any{})
	if got != "3" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Stats(t *testing.T) {
	ctx := map[string]any{
		"rows": []any{
			[]any{"a", 1},
			[]any{"b", 2},
			[]any{"c", 3},
		},
	}
	got := renderWithCtx(t, `{{stats rows 1}}`, ctx)
	if !strings.Contains(got, "min=1") || !strings.Contains(got, "max=3") {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Stats_DefaultColIndex(t *testing.T) {
	// {{stats rows}} with no colIndex — JS defaults to 1. Go now matches.
	ctx := map[string]any{
		"rows": []any{
			[]any{"a", 1},
			[]any{"b", 2},
			[]any{"c", 3},
		},
	}
	got := renderWithCtx(t, `{{stats rows}}`, ctx)
	if !strings.Contains(got, "min=1") || !strings.Contains(got, "max=3") {
		t.Errorf("default colIndex should be 1; got %q", got)
	}
}

// renderWithOpts is like renderWithCtx but lets the test inject a
// custom *Options bundle — needed for the image-URL helpers since the
// generator emits markdown that depends on Options.ImageURL /
// Options.ImageBase64URL being wired by the caller.
func renderWithOpts(t *testing.T, src string, ctx map[string]any, opts *Options) string {
	t.Helper()
	tpl, err := raymond.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	registerHelpers(tpl, opts, map[string]any{})
	out, err := tpl.Exec(ctx)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────
// {{imageURL "key"}}
//
// Resolves an image field's filename to its target-specific URL via
// Options.ImageURL. The generator emits this in url-mode markdown so
// the URL pattern is visible at the source level rather than buried
// inside the polymorphic {{field}} helper.
// ─────────────────────────────────────────────────────────────────────

func TestHelper_ImageURL_RoutesThroughOptions(t *testing.T) {
	tpl := &template.Template{
		Filename: "recepten.yaml",
		Fields:   []template.Field{{Key: "cover", Type: "image"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"cover": "cake.png"})

	opts := &Options{
		ImageURL: func(name string) string { return "/api/images/recepten/" + name },
	}
	got := renderWithOpts(t, `{{imageURL "cover"}}`, ctx, opts)
	if got != "/api/images/recepten/cake.png" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_ImageURL_NilOptionFallsBackToImagesPath(t *testing.T) {
	tpl := &template.Template{
		Filename: "recepten.yaml",
		Fields:   []template.Field{{Key: "cover", Type: "image"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"cover": "cake.png"})

	got := renderWithOpts(t, `{{imageURL "cover"}}`, ctx, &Options{})
	if got != "images/cake.png" {
		t.Errorf("nil ImageURL: want fallback, got %q", got)
	}
}

func TestHelper_ImageURL_EmptyValueReturnsEmpty(t *testing.T) {
	tpl := &template.Template{
		Filename: "recepten.yaml",
		Fields:   []template.Field{{Key: "cover", Type: "image"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"cover": ""})

	got := renderWithOpts(t, `{{imageURL "cover"}}`, ctx, &Options{
		ImageURL: func(name string) string { return "/api/images/recepten/" + name },
	})
	if got != "" {
		t.Errorf("empty value: want empty, got %q", got)
	}
}

func TestHelper_ImageURL_UnknownFieldReturnsEmpty(t *testing.T) {
	tpl := &template.Template{
		Filename: "recepten.yaml",
		Fields:   []template.Field{{Key: "cover", Type: "image"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{})

	got := renderWithOpts(t, `{{imageURL "ghost"}}`, ctx, &Options{
		ImageURL: func(name string) string { return "/api/images/recepten/" + name },
	})
	if got != "" {
		t.Errorf("missing field: want empty, got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// {{imageBase64 "key"}}
//
// Resolves an image field's filename to a `data:<mime>;base64,…` URL
// via Options.ImageBase64URL. Used by the generator's "inline" mode
// so generated markdown is portable (single-file HTML / PDF / wiki
// import where embedding the bytes directly is required).
// ─────────────────────────────────────────────────────────────────────

func TestHelper_ImageBase64_RoutesThroughOptions(t *testing.T) {
	tpl := &template.Template{
		Filename: "recepten.yaml",
		Fields:   []template.Field{{Key: "cover", Type: "image"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"cover": "cake.png"})

	opts := &Options{
		ImageBase64URL: func(name string) string {
			return "data:image/png;base64,FAKE-" + name
		},
	}
	got := renderWithOpts(t, `{{imageBase64 "cover"}}`, ctx, opts)
	if got != "data:image/png;base64,FAKE-cake.png" {
		t.Errorf("got %q", got)
	}
}

func TestHelper_ImageBase64_NilOptionReturnsEmpty(t *testing.T) {
	tpl := &template.Template{
		Filename: "recepten.yaml",
		Fields:   []template.Field{{Key: "cover", Type: "image"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"cover": "cake.png"})

	got := renderWithOpts(t, `{{imageBase64 "cover"}}`, ctx, &Options{})
	if got != "" {
		t.Errorf("nil ImageBase64URL: want empty, got %q", got)
	}
}

func TestHelper_ImageBase64_EmptyValueReturnsEmpty(t *testing.T) {
	tpl := &template.Template{
		Filename: "recepten.yaml",
		Fields:   []template.Field{{Key: "cover", Type: "image"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"cover": ""})

	got := renderWithOpts(t, `{{imageBase64 "cover"}}`, ctx, &Options{
		ImageBase64URL: func(name string) string { return "data:image/png;base64,X" },
	})
	if got != "" {
		t.Errorf("empty value: want empty, got %q", got)
	}
}
