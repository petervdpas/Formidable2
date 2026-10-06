package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func refTpl(md, target string) *template.Template {
	ref := map[string]any{"value": "rule", "label": "TR code", "type": "reference"}
	if target != "" {
		ref[template.ReferenceTargetKey] = target
	}
	return &template.Template{
		Name: "m", Filename: "m.yaml", MarkdownTemplate: md,
		Fields: []template.Field{
			{Key: "mapping", Type: "table", Options: []any{
				map[string]any{"value": "src", "label": "Bron"},
				ref,
			}},
			{Key: "rules", Type: "loopstart"},
			{Key: "tr_code", Type: "text"},
			{Key: "rules", Type: "loopstop"},
		},
	}
}

func refData() map[string]any {
	return map[string]any{
		"mapping": []any{
			[]any{"Basisvariant", "Q-01, C-01"},
			[]any{"A|B", "X-99"},
		},
		"rules": []any{
			map[string]any{"tr_code": "Q-01"},
			map[string]any{"tr_code": "C-01"},
		},
	}
}

func TestFieldHelper_ReferenceTargetEmitsAnchor(t *testing.T) {
	got, err := RenderMarkdown(refData(), refTpl(`{{#loop "rules"}}### {{field "tr_code"}}
{{/loop}}`, "tr_code"), &Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`### <a id="ref-tr_code-q-01"></a>Q-01`, `### <a id="ref-tr_code-c-01"></a>C-01`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestFieldHelper_AnchorKeepsValueEscaped(t *testing.T) {
	data := map[string]any{"rules": []any{map[string]any{"tr_code": "<b>Q</b>"}}}
	got, _ := RenderMarkdown(data, refTpl(`{{#loop "rules"}}{{field "tr_code"}}{{/loop}}`, "tr_code"), &Options{})
	if strings.Contains(got, "<b>") || !strings.Contains(got, "&lt;b&gt;Q&lt;/b&gt;") {
		t.Fatalf("value not escaped: %q", got)
	}
}

func TestFieldHelper_NoAnchorWithoutReferenceOrValue(t *testing.T) {
	got, _ := RenderMarkdown(refData(), refTpl(`{{#loop "rules"}}[{{field "tr_code"}}]{{/loop}}`, ""), &Options{})
	if strings.Contains(got, "<a id=") {
		t.Fatalf("no column targets tr_code, so no anchors: %q", got)
	}
	data := map[string]any{"rules": []any{map[string]any{"tr_code": ""}}}
	got, _ = RenderMarkdown(data, refTpl(`{{#loop "rules"}}[{{field "tr_code"}}]{{/loop}}`, "tr_code"), &Options{})
	if got != "[]" {
		t.Fatalf("empty value, no anchor: %q", got)
	}
}

func TestTableHelper_RendersPipeTableWithReferenceLinks(t *testing.T) {
	got, err := RenderMarkdown(refData(), refTpl(`{{fieldTable "mapping"}}`, "tr_code"), &Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "| Bron | TR code |\n|---|---|\n" +
		"| Basisvariant | [Q-01](#ref-tr_code-q-01), [C-01](#ref-tr_code-c-01) |\n" +
		"| A\\|B | X-99 |\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTableHelper_CodeMatchIsCaseInsensitive(t *testing.T) {
	data := refData()
	data["mapping"] = []any{[]any{"x", "q-01"}}
	got, _ := RenderMarkdown(data, refTpl(`{{fieldTable "mapping"}}`, "tr_code"), &Options{})
	if !strings.Contains(got, "[q-01](#ref-tr_code-q-01)") {
		t.Fatalf("got %q", got)
	}
}

func TestTableHelper_WithoutTargetIsPlainText(t *testing.T) {
	got, _ := RenderMarkdown(refData(), refTpl(`{{fieldTable "mapping"}}`, ""), &Options{})
	if strings.Contains(got, "](#") || !strings.Contains(got, "| Basisvariant | Q-01, C-01 |") {
		t.Fatalf("got %q", got)
	}
}

func TestTableHelper_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		md   string
		data map[string]any
		want string
	}{
		{"empty table", `[{{fieldTable "mapping"}}]`, map[string]any{}, "[]"},
		{"unknown field", `[{{fieldTable "ghost"}}]`, refData(), "[]"},
		{"not a table", `[{{fieldTable "tr_code"}}]`, refData(), "[]"},
		{"short and junk rows", `{{fieldTable "mapping"}}`, map[string]any{"mapping": []any{[]any{"only"}, "junk", []any{nil, 3.5}}},
			"| Bron | TR code |\n|---|---|\n| only |  |\n|  | 3.5 |\n"},
		{"newlines collapse", `{{fieldTable "mapping"}}`, map[string]any{"mapping": []any{[]any{"a\nb", ""}}},
			"| Bron | TR code |\n|---|---|\n| a b |  |\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RenderMarkdown(c.data, refTpl(c.md, "tr_code"), &Options{})
			if err != nil || got != c.want {
				t.Fatalf("got %q err %v, want %q", got, err, c.want)
			}
		})
	}
}

func TestReferences_LinkResolvesInHTML(t *testing.T) {
	md, _ := RenderMarkdown(refData(), refTpl("{{fieldTable \"mapping\"}}\n\n{{#loop \"rules\"}}\n### {{field \"tr_code\"}}\n{{/loop}}", "tr_code"), &Options{})
	html, err := RenderHTML(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="#ref-tr_code-q-01"`) || !strings.Contains(html, `id="ref-tr_code-q-01"`) {
		t.Fatalf("link or target missing in %s", html)
	}
}
