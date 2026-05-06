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

// TestHelper_Table covers the table helper that emits a full markdown
// table — header row, separator, and data rows — joined with single
// newlines. The motivation is to avoid the blank-line-between-rows
// problem the manual {{#each}} approach hits in raymond, which breaks
// goldmark's GFM table parser. See helpers.go for the helper itself.
func TestHelper_Table(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{
			Key: "ingredients", Type: "table",
			Options: []any{
				map[string]any{"value": "name", "label": "Ingredient"},
				map[string]any{"value": "qty", "label": "Quantity"},
			},
		}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{
		"ingredients": []any{
			[]any{"Olive oil", "4 tbsp"},
			[]any{"Lemon", "juice of one"},
		},
	})
	got := renderWithCtx(t, `{{table "ingredients"}}`, ctx)
	want := "| Ingredient | Quantity |\n| --- | --- |\n| Olive oil | 4 tbsp |\n| Lemon | juice of one |\n"
	if got != want {
		t.Errorf("table helper output mismatch.\n got: %q\nwant: %q", got, want)
	}
}

func TestHelper_Table_NoRows(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{
			Key: "ingredients", Type: "table",
			Options: []any{"name", "qty"},
		}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{
		"ingredients": []any{},
	})
	got := renderWithCtx(t, `{{table "ingredients"}}`, ctx)
	want := "| name | qty |\n| --- | --- |\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHelper_Table_UnknownField(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{}}
	ctx := ctxFromTemplate(tpl, map[string]any{})
	got := renderWithCtx(t, `{{table "missing"}}`, ctx)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestHelper_Table_NonTableField(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{Key: "title", Type: "text"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{"title": "x"})
	got := renderWithCtx(t, `{{table "title"}}`, ctx)
	if got != "" {
		t.Errorf("table on non-table field should return empty, got %q", got)
	}
}

func TestHelper_Table_NoOptions(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{Key: "grid", Type: "table"}},
	}
	ctx := ctxFromTemplate(tpl, map[string]any{
		"grid": []any{[]any{"a", "b"}},
	})
	got := renderWithCtx(t, `{{table "grid"}}`, ctx)
	if got != "" {
		t.Errorf("table without options should return empty, got %q", got)
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
