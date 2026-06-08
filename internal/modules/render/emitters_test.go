package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestEmitList(t *testing.T) {
	got := emitList([]any{"a", "b", "c"})
	want := "- a\n- b\n- c"
	if got != want {
		t.Errorf("list = %q, want %q", got, want)
	}
}

func TestEmitList_Empty(t *testing.T) {
	if got := emitList(nil); got != "" {
		t.Errorf("nil list = %q, want empty", got)
	}
	if got := emitList([]any{}); got != "" {
		t.Errorf("empty list = %q, want empty", got)
	}
}

func TestEmitMermaid(t *testing.T) {
	got := emitMermaid("flowchart TD\n  A-->B")
	want := "```mermaid\nflowchart TD\n  A-->B\n```"
	if got != want {
		t.Errorf("mermaid = %q, want %q", got, want)
	}
}

func TestEmitMermaid_Empty(t *testing.T) {
	for _, v := range []any{nil, "", "   ", "\n\n"} {
		if got := emitMermaid(v); got != "" {
			t.Errorf("emitMermaid(%q) = %q, want empty", v, got)
		}
	}
}

func TestEmitMermaid_TrimsTrailingNewlinesBeforeFence(t *testing.T) {
	got := emitMermaid("gantt\n")
	if !strings.HasSuffix(got, "gantt\n```") {
		t.Errorf("mermaid = %q, want a single trailing fence", got)
	}
}

func TestEmitTable(t *testing.T) {
	rows := []any{
		[]any{"r1c1", "r1c2"},
		[]any{"r2c1", "r2c2"},
	}
	got := emitTable(rows)
	want := "| r1c1 | r1c2 |\n| r2c1 | r2c2 |"
	if got != want {
		t.Errorf("table = %q, want %q", got, want)
	}
}

func TestEmitBoolean_Default(t *testing.T) {
	if got := emitBoolean(true, &template.Field{}); got != "True" {
		t.Errorf("default true = %q, want True", got)
	}
	if got := emitBoolean(false, &template.Field{}); got != "False" {
		t.Errorf("default false = %q, want False", got)
	}
	if got := emitBoolean("yes", &template.Field{}); got != "True" {
		t.Errorf(`coerce "yes" = %q, want True`, got)
	}
}

func TestEmitBoolean_CustomOptionsString(t *testing.T) {
	f := &template.Field{Options: []any{"ON", "OFF"}}
	if got := emitBoolean(true, f); got != "ON" {
		t.Errorf("custom true = %q, want ON", got)
	}
	if got := emitBoolean(false, f); got != "OFF" {
		t.Errorf("custom false = %q, want OFF", got)
	}
}

func TestEmitBoolean_CustomOptionsObject(t *testing.T) {
	f := &template.Field{Options: []any{
		map[string]any{"value": "y", "label": "Yes"},
		map[string]any{"value": "n", "label": "No"},
	}}
	if got := emitBoolean(true, f); got != "Yes" {
		t.Errorf("obj true = %q, want Yes", got)
	}
	if got := emitBoolean(false, f); got != "No" {
		t.Errorf("obj false = %q, want No", got)
	}
}

func TestEmitDropdown_StringOptions(t *testing.T) {
	f := &template.Field{Options: []any{"red", "blue"}}
	if got := emitOptionLabel("red", f); got != "red" {
		t.Errorf("got %q, want red", got)
	}
	// missing → fallback to value
	if got := emitOptionLabel("green", f); got != "green" {
		t.Errorf("got %q, want green (fallback)", got)
	}
}

func TestEmitDropdown_ObjectOptions(t *testing.T) {
	f := &template.Field{Options: []any{
		map[string]any{"value": "r", "label": "Red"},
		map[string]any{"value": "b", "label": "Blue"},
	}}
	if got := emitOptionLabel("r", f); got != "Red" {
		t.Errorf("got %q, want Red", got)
	}
}

func TestEmitMultioption(t *testing.T) {
	f := &template.Field{Options: []any{
		map[string]any{"value": "a", "label": "Apple"},
		map[string]any{"value": "b", "label": "Banana"},
	}}
	got := emitMultioption([]any{"a", "b"}, f)
	if got != "Apple, Banana" {
		t.Errorf("got %q, want Apple, Banana", got)
	}
}

func TestEmitMultioption_NotArray(t *testing.T) {
	if got := emitMultioption("nope", &template.Field{}); got != "" {
		t.Errorf("non-array = %q, want empty", got)
	}
}

func TestEmitTags(t *testing.T) {
	got := emitTags([]any{"Hello World", "Go-Lang"}, true)
	want := "#hello-world, #go-lang"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEmitTags_NoHash(t *testing.T) {
	got := emitTags([]any{"foo bar"}, false)
	if got != "foo-bar" {
		t.Errorf("got %q, want foo-bar", got)
	}
}

func TestEmitLink_StringAbsolute(t *testing.T) {
	got := emitLink("https://example.com", &Options{})
	if got != "[https://example.com](https://example.com)" {
		t.Errorf("got %q", got)
	}
}

func TestEmitLink_ObjectWithText(t *testing.T) {
	v := map[string]any{"href": "https://example.com", "text": "Example"}
	got := emitLink(v, &Options{})
	if got != "[Example](https://example.com)" {
		t.Errorf("got %q", got)
	}
}

func TestEmitLink_RelativeWithStrategy(t *testing.T) {
	opts := &Options{
		LinkURL: func(href string) string { return "file:///abs/path/" + href },
	}
	got := emitLink("notes/intro.md", opts)
	if !strings.Contains(got, "file:///abs/path/notes/intro.md") {
		t.Errorf("strategy not applied: %q", got)
	}
}

func TestEmitLink_FileColonPassthrough(t *testing.T) {
	opts := &Options{
		LinkURL: func(href string) string { return "REWRITTEN-" + href },
	}
	got := emitLink("file:///already/abs", opts)
	// file: scheme should not be rewritten
	if strings.Contains(got, "REWRITTEN") {
		t.Errorf("file: was rewritten: %q", got)
	}
}

func TestEmitLink_Empty(t *testing.T) {
	if got := emitLink("", &Options{}); got != "" {
		t.Errorf("empty = %q", got)
	}
	if got := emitLink(nil, &Options{}); got != "" {
		t.Errorf("nil = %q", got)
	}
}

func TestEmitImage_NoStrategy(t *testing.T) {
	got := emitImage("photo.png", &Options{})
	if got != "images/photo.png" {
		t.Errorf("got %q, want images/photo.png", got)
	}
}

func TestEmitImage_WithStrategy(t *testing.T) {
	opts := &Options{
		ImageURL: func(name string) string { return "/storage/x/images/" + name },
	}
	got := emitImage("photo.png", opts)
	if got != "/storage/x/images/photo.png" {
		t.Errorf("got %q", got)
	}
}

func TestEmitImage_NonString(t *testing.T) {
	if got := emitImage(nil, &Options{}); got != "" {
		t.Errorf("nil image = %q", got)
	}
}

func TestEmitFieldValue_Text(t *testing.T) {
	f := &template.Field{Type: "text"}
	if got := emitFieldValue("hello", f, &Options{}); got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestEmitFieldValue_Number(t *testing.T) {
	f := &template.Field{Type: "number"}
	if got := emitFieldValue(42, f, &Options{}); got != "42" {
		t.Errorf("got %q", got)
	}
}

func TestEmitFieldValue_Unknown(t *testing.T) {
	f := &template.Field{Type: "wat"}
	// Unknown types fall back to text (string-ify).
	if got := emitFieldValue("v", f, &Options{}); got != "v" {
		t.Errorf("got %q", got)
	}
}

func TestEmitFieldValue_NilFieldStringifies(t *testing.T) {
	if got := emitFieldValue(7, nil, &Options{}); got != "7" {
		t.Errorf("nil field should stringify, got %q", got)
	}
}

func TestEmitFieldValue_DispatchesByType(t *testing.T) {
	opts := &Options{}
	cases := []struct {
		name  string
		field *template.Field
		value any
		want  string
	}{
		{"list", &template.Field{Type: "list"}, []any{"a", "b"}, "- a\n- b"},
		{"boolean", &template.Field{Type: "boolean"}, true, "True"},
		{"tags", &template.Field{Type: "tags"}, []any{"x"}, "#x"},
		{"mermaid", &template.Field{Type: "mermaid"}, "graph TD", "```mermaid\ngraph TD\n```"},
		{"textarea", &template.Field{Type: "textarea"}, "raw", "raw"},
		{"multioption-nonarray", &template.Field{Type: "multioption"}, "scalar", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := emitFieldValue(c.value, c.field, opts); got != c.want {
				t.Errorf("%s: got %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestStringify_Variants(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"s", "s"},
		{true, "true"},
		{false, "false"},
		{float64(1.5), "1.5"},
		{float32(2.5), "2.5"},
		{int64(9), "9"},
		{uint8(3), "3"},
		{[]string{"a"}, "[a]"},
	}
	for _, c := range cases {
		if got := stringify(c.in); got != c.want {
			t.Errorf("stringify(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruthy_Variants(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{"", false},
		{"false", false},
		{"0", false},
		{"yes", true},
		{0, false},
		{5, true},
		{int64(0), false},
		{int64(2), true},
		{float64(0), false},
		{float64(0.1), true},
		{[]any{}, true},
	}
	for _, c := range cases {
		if got := truthy(c.in); got != c.want {
			t.Errorf("truthy(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}
