package codeformatter

import (
	"strings"
	"testing"
)

// TestFormat_Markdown_UserFCDMTemplate runs the formatter on the exact
// template the user pasted, and verifies (a) the frontmatter survives
// the round-trip with proper nesting intact, and (b) the markdown body
// is byte-identical to the input — Handlebars expressions are NOT
// rewritten, NOT line-wrapped, NOT touched in any way.
func TestFormat_Markdown_UserFCDMTemplate(t *testing.T) {
	frontmatter := `---
cover:
  template: classic
  title: Auto-generated Report
  subtitle: FCDM Entities
  author: Formidable Generator
toc:
  title: Contents
  minDepth: 1
  maxDepth: 3
footer:
  position: center
  showPageNumber: true
style: ""
keywords: [fcdm, entities]
---
`
	body := `
## {{field "name"}}

` + "```" + `csharp
{{#if (fieldRaw "namespace")}}namespace {{field "namespace"}};

{{/if}}public {{field "kind" "value"}} {{field "name"}}{{#if (fieldRaw "base-entity")}} : {{field "base-entity"}}{{#each (fieldRaw "interfaces")}}, {{this}}{{/each}}{{else}}{{#if (length (fieldRaw "interfaces"))}} : {{#each (fieldRaw "interfaces")}}{{this}}{{#unless @last}}, {{/unless}}{{/each}}{{/if}}{{/if}}
{
{{#if (length (fieldRaw "constructor-parameters"))}}    public {{field "name"}}({{#each (fieldRaw "constructor-parameters")}}{{cell this "type" "constructor-parameters"}} {{cell this "name" "constructor-parameters"}}{{#unless @last}}, {{/unless}}{{/each}})
    {
{{#each (fieldRaw "constructor-parameters")}}        {{pascal (cell this "name" "constructor-parameters")}} = {{cell this "name" "constructor-parameters"}};
{{/each}}    }
{{/if}}
}
` + "```" + `
`

	in := frontmatter + body
	out, err := NewManager(nil).Format("markdown", in)
	if err != nil {
		t.Fatal(err)
	}

	// Frontmatter still nested correctly.
	if !strings.Contains(out, "cover:\n  template: classic\n  title: Auto-generated Report") {
		t.Errorf("cover frontmatter mangled:\n%s", out)
	}
	if !strings.Contains(out, "toc:\n  title: Contents\n  minDepth: 1\n  maxDepth: 3") {
		t.Errorf("toc frontmatter mangled:\n%s", out)
	}
	if !strings.Contains(out, "footer:\n  position: center\n  showPageNumber: true") {
		t.Errorf("footer frontmatter mangled:\n%s", out)
	}
	if !strings.Contains(out, "\nstyle: \"\"\n") {
		t.Errorf("top-level style: not preserved as top-level:\n%s", out)
	}

	// Body: every single handlebars expression must survive intact —
	// no line breaks inside {{...}}, no rewrites.
	mustContain := []string{
		`{{#if (fieldRaw "namespace")}}namespace {{field "namespace"}};`,
		`{{#if (fieldRaw "base-entity")}} : {{field "base-entity"}}{{#each (fieldRaw "interfaces")}}`,
		`{{#each (fieldRaw "constructor-parameters")}}{{cell this "type" "constructor-parameters"}} {{cell this "name" "constructor-parameters"}}`,
		`{{pascal (cell this "name" "constructor-parameters")}}`,
	}
	for _, expr := range mustContain {
		if !strings.Contains(out, expr) {
			t.Errorf("handlebars expression altered or wrapped: %q", expr)
		}
	}
	// Specifically reject the line-wrap symptom from the user's paste.
	if strings.Contains(out, "(fieldRaw\n") {
		t.Errorf("body has line-break inside {{...}} expression:\n%s", out)
	}
}
