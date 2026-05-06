package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TestRepro_RecipeTableNoBlankLines proves the recipe table block from
// Examples/templates/recepten.yaml renders without blank lines between
// header/separator and rows (the goldmark GFM table parser requires
// contiguous lines). Catches regressions in the forked raymond's
// standalone-tag whitespace handling.
func TestRepro_RecipeTableNoBlankLines(t *testing.T) {
	tpl := &template.Template{
		MarkdownTemplate: `## Ingredienten

{{#with (fieldMeta "ingredienten" "options") as |headers|}}
|{{#each headers}}{{label}}{{^label}}{{value}}{{/label}} |{{/each}}
|{{#each headers}}--|{{/each}}
  {{/with}}
  {{#each (fieldRaw "ingredienten")}}
|{{#each this}}{{this}} |{{/each}}
{{/each}}

## Bereiding`,
		Fields: []template.Field{
			{Key: "ingredienten", Type: "table", Options: []any{
				map[string]any{"value": "name", "label": "Ingrediënt"},
				map[string]any{"value": "qty", "label": "Hoeveelheid / Gewicht"},
			}},
		},
	}
	out, err := RenderMarkdown(map[string]any{
		"ingredienten": []any{
			[]any{"Krieltjes in schil", "10-12 stuks"},
			[]any{"Sjalotten", "3-4 middelgroot"},
		},
	}, tpl, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Locate the table block and assert no blank lines between rows.
	idx := strings.Index(out, "|Ingrediënt")
	end := strings.Index(out[idx:], "## Bereiding")
	if idx < 0 || end < 0 {
		t.Fatalf("table block not found in output:\n%s", out)
	}
	tableBlock := strings.TrimRight(out[idx:idx+end], "\n ")
	if strings.Contains(tableBlock, "\n\n") {
		t.Errorf("table block must not contain blank lines; got:\n%s", tableBlock)
	}
	for _, want := range []string{
		"|Ingrediënt |Hoeveelheid / Gewicht |",
		"|--|--|",
		"|Krieltjes in schil |10-12 stuks |",
		"|Sjalotten |3-4 middelgroot |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing line %q in:\n%s", want, out)
		}
	}
}
